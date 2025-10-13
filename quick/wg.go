package quick

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// Up sets and configures the wg interface. Mostly equivalent to `wg-quick up iface`
func Up(cfg *Config, iface string, logger zerolog.Logger) error {
	_, err := netlink.LinkByName(iface)
	if err == nil {
		return os.ErrExist
	}
	var linkNotFoundError netlink.LinkNotFoundError
	if !errors.As(err, &linkNotFoundError) {
		return err
	}

	for _, dns := range cfg.DNS {
		if err := execSh("resolvconf -a tun.%i -m 0 -x", iface, logger, fmt.Sprintf("nameserver %s\n", dns)); err != nil {
			return err
		}
	}

	if len(cfg.PreUp) > 0 {
		for _, cmd := range cfg.PreUp {
			if err := execSh(cmd, iface, logger); err != nil {
				return err
			}
		}
		logger.Info().Msg("applied pre-up command")
	}

	if err := Sync(cfg, iface, logger); err != nil {
		return err
	}

	if len(cfg.PostUp) > 0 {
		for _, cmd := range cfg.PostUp {
			if err := execSh(cmd, iface, logger); err != nil {
				return err
			}
		}
		logger.Info().Msg("applied post-up command")
	}

	return nil
}

// Down destroys the wg interface. Mostly equivalent to `wg-quick down iface`
func Down(cfg *Config, iface string, logger zerolog.Logger) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}

	if len(cfg.DNS) > 1 {
		if err := execSh("resolvconf -d tun.%s", iface, logger); err != nil {
			return err
		}
	}

	if len(cfg.PreDown) > 0 {
		for _, cmd := range cfg.PreDown {
			if err := execSh(cmd, iface, logger); err != nil {
				return err
			}
		}
		logger.Info().Msg("applied pre-down command")
	}

	if err := netlink.LinkDel(link); err != nil {
		return err
	}
	logger.Info().Msg("link deleted")

	if len(cfg.PostDown) > 0 {
		for _, cmd := range cfg.PostDown {
			if err := execSh(cmd, iface, logger); err != nil {
				return err
			}
		}
		logger.Info().Msg("applied post-down command")
	}

	return nil
}

func execSh(command string, iface string, logger zerolog.Logger, stdin ...string) error {
	cmd := exec.Command("sh", "-ce", strings.ReplaceAll(command, "%i", iface))
	if len(stdin) > 0 {
		logger = logger.With().Str("stdin", strings.Join(stdin, "")).Logger()
		b := &bytes.Buffer{}
		for _, ln := range stdin {
			if _, err := fmt.Fprint(b, ln); err != nil {
				return err
			}
		}
		cmd.Stdin = b
	}
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		if err != nil {
			logger.Err(err).Msgf("failed to execute %s:\n%s", cmd.Args, out)
			return err
		}
		logger.Info().Msgf("executed %s:\n%s", cmd.Args, out)
	} else {
		if err != nil {
			logger.Err(err).Msgf("failed to execute %s", cmd.Args)
			return err
		}
		logger.Info().Msgf("executed %s", cmd.Args)
	}
	return nil
}

// Sync the config to the current setup for given interface
// It perform 4 operations:
// * SyncLink --> makes sure link is up and type wireguard
// * SyncWireguardDevice --> configures allowedIP & other wireguard specific settings
// * SyncAddress --> synces linux addresses bounded to this interface
// * SyncRoutes --> synces all allowedIP routes to route to this interface, if Table is not off
func Sync(cfg *Config, iface string, logger zerolog.Logger) error {
	link, err := SyncLink(cfg, iface, logger)
	if err != nil {
		logger.Err(err).Msg("cannot sync wireguard link")
		return err
	}
	logger.Info().Msg("synced link")

	if err := SyncWireguardDevice(cfg, link, logger); err != nil {
		logger.Err(err).Msg("cannot sync wireguard link")
		return err
	}
	logger.Info().Msg("synced link")

	if err := SyncAddress(cfg, link, logger); err != nil {
		logger.Err(err).Msg("cannot sync addresses")
		return err
	}
	logger.Info().Msg("synced addresses")

	if cfg.Table != nil {
		var managedRoutes []net.IPNet
		for _, peer := range cfg.Peers {
			for _, rt := range peer.AllowedIPs {
				managedRoutes = append(managedRoutes, rt)
			}
		}
		if err := SyncRoutes(cfg, link, managedRoutes, logger); err != nil {
			logger.Err(err).Msg("cannot sync routes")
			return err
		}
		logger.Info().Msg("synced routed")
	} else {
		logger.Info().Msg("Table=off, skip route sync")
	}

	logger.Info().Msg("Successfully synced device")
	return nil

}

// SyncWireguardDevice syncs wireguard vpn setting on the given link. It does not set routes/addresses beyond wg internal crypto-key routing, only handles wireguard specific settings
func SyncWireguardDevice(cfg *Config, link netlink.Link, logger zerolog.Logger) error {
	if err := client.ConfigureDevice(link.Attrs().Name, cfg.Config); err != nil {
		logger.Err(err).Msg("cannot configure device")
		return err
	}
	return nil
}

// SyncLink syncs link state with the config. It does not sync Wireguard settings, just makes sure the device is up and type wireguard
func SyncLink(cfg *Config, iface string, logger zerolog.Logger) (netlink.Link, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		var linkNotFoundError netlink.LinkNotFoundError
		if !errors.As(err, &linkNotFoundError) {
			logger.Err(err).Msg("cannot read link")
			return nil, err
		}
		logger.Info().Msg("link not found, creating")

		if cfg.WgBin == "" {
			wgLink := &netlink.GenericLink{
				LinkAttrs: netlink.LinkAttrs{
					Name: iface,
					MTU:  cfg.MTU,
				},
				LinkType: "wireguard",
			}
			if err := netlink.LinkAdd(wgLink); err != nil {
				logger.Err(err).Msg("cannot create link")
				return nil, err
			}
		} else {
			logger.Info().Msgf("using %s to create link", cfg.WgBin)
			output, err := exec.Command(cfg.WgBin, iface).CombinedOutput()
			if err != nil {
				logger.Err(err).Str("output", string(output)).Msg("cannot create link")
				return nil, err
			}
		}

		link, err = netlink.LinkByName(iface)
		if err != nil {
			logger.Err(err).Msg("cannot read link")
			return nil, err
		}
	}
	if err := netlink.LinkSetUp(link); err != nil {
		logger.Err(err).Msg("cannot set link up")
		return nil, err
	}
	logger.Info().Msg("set device up")
	return link, nil
}

// SyncAddress adds/deletes all lind assigned IPV4 addressed as specified in the config
func SyncAddress(cfg *Config, link netlink.Link, logger zerolog.Logger) error {
	addrs, err := netlink.AddrList(link, syscall.AF_INET)
	if err != nil {
		logger.Err(err).Msg("cannot read link address")
		return err
	}

	// nil addr means I've used it
	presentAddresses := make(map[string]netlink.Addr, 0)
	for _, addr := range addrs {
		logger.Debug().Str("addr", addr.IPNet.String()).Str("label", addr.Label).Msg("found existing address")
		presentAddresses[addr.IPNet.String()] = addr
	}

	for _, addr := range cfg.Address {
		logger := logger.With().Str("addr", addr.String()).Logger()
		_, present := presentAddresses[addr.String()]
		presentAddresses[addr.String()] = netlink.Addr{} // mark as present
		if present {
			logger.Info().Msg("address present")
			continue
		}
		if err := netlink.AddrAdd(link, &netlink.Addr{
			IPNet: &addr,
			Label: cfg.AddressLabel,
		}); err != nil {
			if !errors.Is(err, syscall.EEXIST) {
				logger.Err(err).Msg("cannot add addr")
				return err
			}
		}
		logger.Info().Msg("address added")
	}

	for _, addr := range presentAddresses {
		if addr.IPNet == nil {
			continue
		}
		logger := logger.With().Str("addr", addr.IPNet.String()).Str("label", addr.Label).Logger()
		if err := netlink.AddrDel(link, &addr); err != nil {
			logger.Err(err).Msg("cannot delete addr")
			return err
		}
		logger.Info().Msg("addr deleted")
	}
	return nil
}

func fillRouteDefaults(rt *netlink.Route) {
	// fill defaults
	if rt.Table == 0 {
		rt.Table = unix.RT_CLASS_MAIN
	}

	if rt.Protocol == 0 {
		rt.Protocol = unix.RTPROT_BOOT
	}

	if rt.Type == 0 {
		rt.Type = unix.RTN_UNICAST
	}
}

// SyncRoutes adds/deletes all route assigned IPV4 addressed as specified in the config
func SyncRoutes(cfg *Config, link netlink.Link, managedRoutes []net.IPNet, logger zerolog.Logger) error {
	if cfg.Table == nil {
		return nil
	}
	var wantedRoutes = make(map[string][]netlink.Route, len(managedRoutes))
	presentRoutes, err := netlink.RouteList(link, syscall.AF_INET)
	if err != nil {
		logger.Err(err).Msg("cannot read existing routes")
		return err
	}

	for _, rt := range managedRoutes {
		rt := rt // make copy
		logger.Debug().Str("dst", rt.String()).Msg("managing route")

		nrt := netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       &rt,
			Table:     *cfg.Table,
			Protocol:  cfg.RouteProtocol,
			Priority:  cfg.RouteMetric}
		fillRouteDefaults(&nrt)
		wantedRoutes[rt.String()] = append(wantedRoutes[rt.String()], nrt)
	}

	for _, rtLst := range wantedRoutes {
		for _, rt := range rtLst {
			rt := rt // make copy
			log := logger.With().
				Str("route", rt.Dst.String()).
				Int("protocol", rt.Protocol).
				Int("table", rt.Table).
				Int("type", rt.Type).
				Int("metric", rt.Priority).
				Logger()
			if err := netlink.RouteReplace(&rt); err != nil {
				log.Err(err).Msg("cannot add/replace route")
				return err
			}
			log.Info().Msg("route added/replaced")
		}
	}

	checkWanted := func(rt netlink.Route) bool {
		for _, candidateRt := range wantedRoutes[rt.Dst.String()] {
			if rt.Equal(candidateRt) {
				return true
			}
		}
		return false
	}

	for _, rt := range presentRoutes {
		log := logger.With().
			Str("route", rt.Dst.String()).
			Int("protocol", rt.Protocol).
			Int("table", rt.Table).
			Int("type", rt.Type).
			Int("metric", rt.Priority).
			Logger()
		if !(rt.Table == *cfg.Table || (*cfg.Table == 0 && rt.Table == unix.RT_CLASS_MAIN)) {
			log.Debug().Msg("wrong table for route, skipping")
			continue
		}

		if !(rt.Protocol == cfg.RouteProtocol) {
			log.Info().Msgf("skipping route deletion, not owned by this daemon")
			continue
		}

		if checkWanted(rt) {
			log.Debug().Msg("route wanted, skipping deleting")
			continue
		}

		if err := netlink.RouteDel(&rt); err != nil {
			log.Err(err).Msg("cannot delete route")
			return err
		}
		log.Info().Msg("route deleted")
	}
	return nil
}
