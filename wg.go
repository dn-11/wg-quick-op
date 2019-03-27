package wgquick

import (
	"github.com/mdlayher/wireguardctrl"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"syscall"
)


const (
	defaultRoutingTable = 254
)

// Sync the config to the current setup for given interface
func (cfg *Config) Sync(iface string, logger logrus.FieldLogger) error {
	log := logger.WithField("iface", iface)
	link, err := netlink.LinkByName(iface)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); !ok {
			log.WithError(err).Error("cannot read link")
			return err
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
			return err
		}

		link, err = netlink.LinkByName(iface)
		if err != nil {
			log.WithError(err).Error("cannot read link")
			return err
		}
	}
	if err := netlink.LinkSetUp(link); err != nil {
		log.WithError(err).Error("cannot set link up")
		return err
	}
	log.Info("set device up")

	cl, err := wireguardctrl.New()
	if err != nil {
		log.Error(err, "cannot setup wireguard device")
		return err
	}

	if err := cl.ConfigureDevice(iface, cfg.Config); err != nil {
		log.WithError(err).Error("cannot configure device")
		return err
	}

	if err := syncAddress(link, cfg, log); err != nil {
		log.Error(err, "cannot sync addresses")
		return err
	}

	if err := syncRoutes(link, cfg, log); err != nil {
		log.Error(err, "cannot sync routes")
		return err
	}

	log.Info("Successfully setup device")
	return nil

}

func syncAddress(link netlink.Link, cfg *Config, log logrus.FieldLogger) error {
	addrs, err := netlink.AddrList(link, syscall.AF_INET)
	if err != nil {
		log.Error(err, "cannot read link address")
		return err
	}

	// nil addr means I've used it
	presentAddresses := make(map[string]*netlink.Addr, 0)
	for _, addr := range addrs {
		presentAddresses[addr.IPNet.String()] = &addr
	}

	for _, addr := range cfg.Address {
		log := log.WithField("addr", addr)
		_, present := presentAddresses[addr.String()]
		presentAddresses[addr.String()] = nil // mark as present
		if present {
			log.Info("address present")
			continue
		}
		if err := netlink.AddrAdd(link, &netlink.Addr{
			IPNet: addr,
		}); err != nil {
			log.WithError(err).Error("cannot add addr")
			return err
		}
		log.Info("address added")
	}

	for _, addr := range presentAddresses {
		if addr == nil {
			continue
		}
		log := log.WithField("addr", addr.IPNet.String())
		if err := netlink.AddrDel(link, addr); err != nil {
			log.WithError(err).Error("cannot delete addr")
			return err
		}
		log.Info("addr deleted")
	}
	return nil
}

func syncRoutes(link netlink.Link, cfg *Config, log logrus.FieldLogger) error {
	routes, err := netlink.RouteList(link, syscall.AF_INET)
	if err != nil {
		log.Error(err, "cannot read existing routes")
		return err
	}

	presentRoutes := make(map[string]*netlink.Route, 0)
	for _, r := range routes {
		log := log.WithField("route", r.Dst.String())
		if r.Table == cfg.Table || (cfg.Table == 0 && r.Table == defaultRoutingTable) {
			presentRoutes[r.Dst.String()] = &r
			log.WithField("table", r.Table).Debug("detected existing route")
		} else {
			log.Debug("wrong table for route, skipping")
		}
	}

	for _, peer := range cfg.Peers {
		for _, rt := range peer.AllowedIPs {
			_, present := presentRoutes[rt.String()]
			presentRoutes[rt.String()] = nil // mark as visited
			log := log.WithField("route", rt.String())
			if present {
				log.Info("route present")
				continue
			}
			if err := netlink.RouteAdd(&netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       &rt,
				Table:     cfg.Table,
			}); err != nil {
				log.WithError(err).Error("cannot setup route")
				return err
			}
			log.Info("route added")
		}
	}

	// Clean extra routes
	for _, rt := range presentRoutes {
		if rt == nil { // skip visited routes
			continue
		}
		log := log.WithField("route", rt.Dst.String())
		log.Info("extra manual route found")
		if err := netlink.RouteDel(rt); err != nil {
			log.WithError(err).Error("cannot setup route")
			return err
		}
		log.Info("route deleted")
	}
	return nil
}
