package wgquick

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/mdlayher/wireguardctrl"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const (
	defaultRoutingTable = 254

	// From linux/rtnetlink.h
	RTPROT_UNSPEC   = 0
	RTPROT_REDIRECT = 1 /* Route installed by ICMP redirects; not used by current IPv4 */
	RTPROT_KERNEL   = 2 /* Route installed by kernel		*/
	RTPROT_BOOT     = 3 /* Route installed during boot		*/
	RTPROT_STATIC   = 4 /* Route installed by administrator	*/

	/* Values of protocol >= RTPROT_STATIC are not interpreted by kernel;
	   they are just passed from user and back as is.
	   It will be used by hypothetical multiple routing daemons.
	   Note that protocol values should be standardized in order to
	   avoid conflicts.
	*/

	RTPROT_GATED    = 8   /* Apparently, GateD */
	RTPROT_RA       = 9   /* RDISC/ND router advertisements */
	RTPROT_MRT      = 10  /* Merit MRT */
	RTPROT_ZEBRA    = 11  /* Zebra */
	RTPROT_BIRD     = 12  /* BIRD */
	RTPROT_DNROUTED = 13  /* DECnet routing daemon */
	RTPROT_XORP     = 14  /* XORP */
	RTPROT_NTK      = 15  /* Netsukuku */
	RTPROT_DHCP     = 16  /* DHCP client */
	RTPROT_MROUTED  = 17  /* Multicast daemon */
	RTPROT_BABEL    = 42  /* Babel daemon */
	RTPROT_BGP      = 186 /* BGP Routes */
	RTPROT_ISIS     = 187 /* ISIS Routes */
	RTPROT_OSPF     = 188 /* OSPF Routes */
	RTPROT_RIP      = 189 /* RIP Routes */
	RTPROT_EIGRP    = 192 /* EIGRP Routes */
)

// Up sets and configures the wg interface. Mostly equivalent to `wg-quick up iface`
func Up(cfg *Config, iface string, logger logrus.FieldLogger) error {
	log := logger.WithField("iface", iface)
	_, err := netlink.LinkByName(iface)
	if err == nil {
		return os.ErrExist
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return err
	}

	for _, dns := range cfg.DNS {
		if err := execSh("resolvconf -a tun.%i -m 0 -x", iface, log, fmt.Sprintf("nameserver %s\n", dns)); err != nil {
			return err
		}
	}

	if cfg.PreUp != "" {
		if err := execSh(cfg.PreUp, iface, log); err != nil {
			return err
		}
		log.Infoln("applied pre-up command")
	}
	if err := Sync(cfg, iface, logger); err != nil {
		return err
	}

	if cfg.PostUp != "" {
		if err := execSh(cfg.PostUp, iface, log); err != nil {
			return err
		}
		log.Infoln("applied post-up command")
	}
	return nil
}

// Down destroys the wg interface. Mostly equivalent to `wg-quick down iface`
func Down(cfg *Config, iface string, logger logrus.FieldLogger) error {
	log := logger.WithField("iface", iface)
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}

	if len(cfg.DNS) > 1 {
		if err := execSh("resolvconf -d tun.%s", iface, log); err != nil {
			return err
		}
	}

	if cfg.PreDown != "" {
		if err := execSh(cfg.PreDown, iface, log); err != nil {
			return err
		}
		log.Infoln("applied pre-down command")
	}

	if err := netlink.LinkDel(link); err != nil {
		return err
	}
	log.Infoln("link deleted")
	if cfg.PostDown != "" {
		if err := execSh(cfg.PostDown, iface, log); err != nil {
			return err
		}
		log.Infoln("applied post-down command")
	}
	return nil
}

func execSh(command string, iface string, log logrus.FieldLogger, stdin ...string) error {
	cmd := exec.Command("sh", "-ce", strings.ReplaceAll(command, "%i", iface))
	if len(stdin) > 0 {
		log = log.WithField("stdin", strings.Join(stdin, ""))
		b := &bytes.Buffer{}
		for _, ln := range stdin {
			if _, err := fmt.Fprint(b, ln); err != nil {
				return err
			}
		}
		cmd.Stdin = b
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.WithError(err).Errorf("failed to execute %s:\n%s", cmd.Args, out)
		return err
	}
	log.Infof("executed %s:\n%s", cmd.Args, out)
	return nil
}

// Sync the config to the current setup for given interface
// It perform 4 operations:
// * SyncLink --> makes sure link is up and type wireguard
// * SyncWireguardDevice --> configures allowedIP & other wireguard specific settings
// * SyncAddress --> synces linux addresses bounded to this interface
// * SyncRoutes --> synces all allowedIP routes to route to this interface
func Sync(cfg *Config, iface string, logger logrus.FieldLogger) error {
	log := logger.WithField("iface", iface)

	link, err := SyncLink(cfg, iface, log)
	if err != nil {
		log.WithError(err).Errorln("cannot sync wireguard link")
		return err
	}
	log.Info("synced link")

	if err := SyncWireguardDevice(cfg, link, log); err != nil {
		log.WithError(err).Errorln("cannot sync wireguard link")
		return err
	}
	log.Info("synced link")

	if err := SyncAddress(cfg, link, log); err != nil {
		log.WithError(err).Errorln("cannot sync addresses")
		return err
	}
	log.Info("synced addresss")

	var managedRoutes []net.IPNet
	for _, peer := range cfg.Peers {
		for _, rt := range peer.AllowedIPs {
			managedRoutes = append(managedRoutes, rt)
		}
	}
	if err := SyncRoutes(cfg, link, managedRoutes, log); err != nil {
		log.WithError(err).Errorln("cannot sync routes")
		return err
	}
	log.Info("synced routed")
	log.Info("Successfully synced device")
	return nil

}

// SyncWireguardDevice synces wireguard vpn setting on the given link. It does not set routes/addresses beyond wg internal crypto-key routing, only handles wireguard specific settings
func SyncWireguardDevice(cfg *Config, link netlink.Link, log logrus.FieldLogger) error {
	cl, err := wireguardctrl.New()
	if err != nil {
		log.WithError(err).Errorln("cannot setup wireguard device")
		return err
	}
	if err := cl.ConfigureDevice(link.Attrs().Name, cfg.Config); err != nil {
		log.WithError(err).Error("cannot configure device")
		return err
	}
	return nil
}

// SyncLink synces link state with the config. It does not sync Wireguard settings, just makes sure the device is up and type wireguard
func SyncLink(cfg *Config, iface string, log logrus.FieldLogger) (netlink.Link, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); !ok {
			log.WithError(err).Error("cannot read link")
			return nil, err
		}
		log.Info("link not found, creating")
		wgLink := &netlink.GenericLink{
			LinkAttrs: netlink.LinkAttrs{
				Name: iface,
				MTU:  cfg.MTU,
			},
			LinkType: "wireguard",
		}
		if err := netlink.LinkAdd(wgLink); err != nil {
			log.WithError(err).Error("cannot create link")
			return nil, err
		}

		link, err = netlink.LinkByName(iface)
		if err != nil {
			log.WithError(err).Error("cannot read link")
			return nil, err
		}
	}
	if err := netlink.LinkSetUp(link); err != nil {
		log.WithError(err).Error("cannot set link up")
		return nil, err
	}
	log.Info("set device up")
	return link, nil
}

// SyncAddress adds/deletes all lind assigned IPV4 addressed as specified in the config
func SyncAddress(cfg *Config, link netlink.Link, log logrus.FieldLogger) error {
	addrs, err := netlink.AddrList(link, syscall.AF_INET)
	if err != nil {
		log.Error(err, "cannot read link address")
		return err
	}

	// nil addr means I've used it
	presentAddresses := make(map[string]netlink.Addr, 0)
	for _, addr := range addrs {
		log.WithFields(map[string]interface{}{
			"addr":  fmt.Sprint(addr.IPNet),
			"label": addr.Label,
		}).Debugf("found existing address: %v", addr)
		presentAddresses[addr.IPNet.String()] = addr
	}

	for _, addr := range cfg.Address {
		log := log.WithField("addr", addr.String())
		_, present := presentAddresses[addr.String()]
		presentAddresses[addr.String()] = netlink.Addr{} // mark as present
		if present {
			log.Info("address present")
			continue
		}
		if err := netlink.AddrAdd(link, &netlink.Addr{
			IPNet: &addr,
			Label: cfg.AddressLabel,
		}); err != nil {
			log.WithError(err).Error("cannot add addr")
			return err
		}
		log.Info("address added")
	}

	for _, addr := range presentAddresses {
		if addr.IPNet == nil {
			continue
		}
		log := log.WithFields(map[string]interface{}{
			"addr":  addr.IPNet.String(),
			"label": addr.Label,
		})
		if err := netlink.AddrDel(link, &addr); err != nil {
			log.WithError(err).Error("cannot delete addr")
			return err
		}
		log.Info("addr deleted")
	}
	return nil
}

// SyncRoutes adds/deletes all route assigned IPV4 addressed as specified in the config
func SyncRoutes(cfg *Config, link netlink.Link, managedRoutes []net.IPNet, log logrus.FieldLogger) error {
	routes, err := netlink.RouteList(link, syscall.AF_INET)
	if err != nil {
		log.Error(err, "cannot read existing routes")
		return err
	}

	presentRoutes := make(map[string]netlink.Route, 0)
	for _, r := range routes {
		log := log.WithFields(map[string]interface{}{
			"route":    r.Dst.String(),
			"protocol": r.Protocol,
			"table":    r.Table,
			"type":     r.Type,
		})
		log.Debugf("detected existing route: %v", r)
		if !(r.Table == cfg.Table || (cfg.Table == 0 && r.Table == defaultRoutingTable)) {
			log.Debug("wrong table for route, skipping")
			continue
		}
		presentRoutes[r.Dst.String()] = r
		log.Debug("added route to consideration")
	}

	for _, rt := range managedRoutes {
		log := log.WithField("route", rt.String())
		route, present := presentRoutes[rt.String()]
		presentRoutes[rt.String()] = netlink.Route{} // mark as visited
		if present {
			if route.Dst != nil && route.Protocol != cfg.RouteProtocol {
				log.Warnf("route present; proto=%d != defined root proto=%d", route.Protocol, cfg.RouteProtocol)
			} else {
				log.Info("route present")
			}
			continue
		}
		if err := netlink.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       &rt,
			Table:     cfg.Table,
			Protocol:  cfg.RouteProtocol,
		}); err != nil {
			log.WithError(err).Error("cannot setup route")
			return err
		}
		log.Info("route added")
	}

	// Clean extra routes
	for _, rt := range presentRoutes {
		if rt.Dst == nil { // skip visited routes
			continue
		}
		log := log.WithFields(map[string]interface{}{
			"route":    rt.Dst.String(),
			"protocol": rt.Protocol,
			"table":    rt.Table,
			"type":     rt.Type,
		})
		log.Info("extra manual route found")
		// RTPROT_BOOT is default one when other proto isn't defined
		if !(rt.Protocol == cfg.RouteProtocol || rt.Protocol == RTPROT_BOOT && cfg.RouteProtocol == 0) {
			log.Infof("skipping route deletion, not owned by this daemon")
			continue
		}
		if err := netlink.RouteDel(&rt); err != nil {
			log.WithError(err).Error("cannot delete route")
			return err
		}
		log.Info("route deleted")
	}
	return nil
}
