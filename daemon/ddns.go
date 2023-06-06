package daemon

import (
	"github.com/BaiMeow/wg-quick-op/quick"
	"github.com/sirupsen/logrus"
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
		logrus.WithField("iface", iface).WithError(err).Error("failed to get unresolved unresolved Endpoint")
	}
	ddnsConfig.unresolvedEndpoints = endpoints
	return &ddnsConfig, nil
}
