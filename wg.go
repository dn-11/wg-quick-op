package wgctl

import (
	"bytes"
	"encoding/base64"
	"net"
	"syscall"
	"text/template"

	"github.com/mdlayher/wireguardctrl"
	"github.com/mdlayher/wireguardctrl/wgtypes"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Config struct {
	wgtypes.Config

	// Address list of IP (v4 or v6) addresses (optionally with CIDR masks) to be assigned to the interface. May be specified multiple times.
	Address []*net.IPNet

	// list of IP (v4 or v6) addresses to be set as the interface’s DNS servers. May be specified multiple times. Upon bringing the interface up, this runs ‘resolvconf -a tun.INTERFACE -m 0 -x‘ and upon bringing it down, this runs ‘resolvconf -d tun.INTERFACE‘. If these particular invocations of resolvconf(8) are undesirable, the PostUp and PostDown keys below may be used instead.
	// Currently unsupported
	DNS []net.IP
	// —if not specified, the MTU is automatically determined from the endpoint addresses or the system default route, which is usually a sane choice. However, to manually specify an MTU to override this automatic discovery, this value may be specified explicitly.
	MTU int

	// Table — Controls the routing table to which routes are added.
	Table int

	// PreUp, PostUp, PreDown, PostDown — script snippets which will be executed by bash(1) before/after setting up/tearing down the interface, most commonly used to configure custom DNS options or firewall rules. The special string ‘%i’ is expanded to INTERFACE. Each one may be specified multiple times, in which case the commands are executed in order.

	// Currently unsupported
	PreUp    string
	PostUp   string
	PreDown  string
	PostDown string

	// SaveConfig — if set to ‘true’, the configuration is saved from the current state of the interface upon shutdown.
	// Currently unsupported
	SaveConfig bool
}

func (cfg *Config) String() string {
	b, err := cfg.MarshalText()
	if err != nil {
		panic(err)
	}
	return string(b)
}

var cfgTemplate = template.Must(
	template.
		New("wg-cfg").
		Funcs(template.FuncMap(map[string]interface{}{"wgKey": serializeKey})).
		Parse(wgtypeTemplateSpec))

func (cfg *Config) MarshalText() (text []byte, err error) {
	buff := &bytes.Buffer{}
	if err := cfgTemplate.Execute(buff, cfg); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

const wgtypeTemplateSpec = `[Interface]
{{- range .Address }}
Address = {{ . }}
{{- end }}
{{- range .DNS }}
DNS = {{ . }}
{{- end }}
PrivateKey = {{ .PrivateKey | wgKey }}
{{- if .ListenPort }}{{ "\n" }}ListenPort = {{ .ListenPort }}{{ end }}
{{- if .MTU }}{{ "\n" }}MTU = {{ .MTU }}{{ end }}
{{- if .Table }}{{ "\n" }}Table = {{ .Table }}{{ end }}
{{- if .PreUp }}{{ "\n" }}PreUp = {{ .PreUp }}{{ end }}
{{- if .PostUp }}{{ "\n" }}Table = {{ .Table }}{{ end }}
{{- if .PreDown }}{{ "\n" }}PreDown = {{ .PreDown }}{{ end }}
{{- if .PostDown }}{{ "\n" }}PostDown = {{ .PostDown }}{{ end }}
{{- if .SaveConfig }}{{ "\n" }}SaveConfig = {{ .SaveConfig }}{{ end }}
{{- range .Peers }}
{{- "\n" }}
[Peer]
PublicKey = {{ .PublicKey | wgKey }}
AllowedIps = {{ range $i, $el := .AllowedIPs }}{{if $i}}, {{ end }}{{ $el }}{{ end }}
{{- if .Endpoint }}{{ "\n" }}Endpoint = {{ .Endpoint }}{{ end }}
{{- end }}
`

func serializeKey(key *wgtypes.Key) string {
	return base64.StdEncoding.EncodeToString(key[:])
}

// Parses base64 encoded key
func ParseKey(key string) (wgtypes.Key, error) {
	var pkey wgtypes.Key
	pkeySlice, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return pkey, err
	}
	copy(pkey[:], pkeySlice[:])
	return pkey, nil
}

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

	presentAddresses := make(map[string]int, 0)
	for _, addr := range addrs {
		presentAddresses[addr.IPNet.String()] = 1
	}

	for _, addr := range cfg.Address {
		log := log.WithField("addr", addr)
		_, present := presentAddresses[addr.String()]
		presentAddresses[addr.String()] = 2
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

	for addr, p := range presentAddresses {
		log := log.WithField("addr", addr)
		if p < 2 {
			nlAddr, err := netlink.ParseAddr(addr)
			if err != nil {
				log.WithError(err).Error("cannot parse del addr")
				return err
			}
			if err := netlink.AddrAdd(link, nlAddr); err != nil {
				log.WithError(err).Error("cannot delete addr")
				return err
			}
			log.Info("addr deleted")
		}
	}
	return nil
}

func syncRoutes(link netlink.Link, cfg *Config, log logrus.FieldLogger) error {
	routes, err := netlink.RouteList(link, syscall.AF_INET)
	if err != nil {
		log.Error(err, "cannot read existing routes")
		return err
	}

	presentRoutes := make(map[string]int, 0)
	for _, r := range routes {
		log := log.WithField("route", r.Dst.String())
		if r.Table == cfg.Table {
			presentRoutes[r.Dst.String()] = 1
			log.Debug("detected existing route")
		} else {
			log.Debug("wrong table for route, skipping")
		}
	}

	for _, peer := range cfg.Peers {
		for _, rt := range peer.AllowedIPs {
			_, present := presentRoutes[rt.String()]
			presentRoutes[rt.String()] = 2
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
	for rtStr, p := range presentRoutes {
		_, rt, err := net.ParseCIDR(rtStr)
		log := log.WithField("route", rt.String())
		if err != nil {
			log.WithError(err).Error("cannot parse route")
			return err
		}
		if p < 2 {
			log.Info("extra manual route found")
			if err := netlink.RouteDel(&netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       rt,
				Table:     cfg.Table,
			}); err != nil {
				log.WithError(err).Error("cannot setup route")
				return err
			}
			log.Info("route deleted")
		}
	}
	return nil
}
