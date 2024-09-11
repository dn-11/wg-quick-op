package daemon

import (
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/rs/zerolog/log"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ddns struct {
	cfg                 *quick.Config
	name                string
	unresolvedEndpoints map[wgtypes.Key]string
}

func newDDNS(iface string) (*ddns, error) {
	var ddnsConfig ddns
	ddnsConfig.name = iface
	cfg, err := quick.GetConfig(iface)
	if err != nil {
		return nil, err
	}
	ddnsConfig.cfg = cfg

	endpoints, err := quick.GetUnresolvedEndpoints(iface)
	if err != nil {
		log.Err(err).Str("iface", iface).Msg("failed to get unresolved unresolved Endpoint")
	}
	ddnsConfig.unresolvedEndpoints = endpoints
	return &ddnsConfig, nil
}
