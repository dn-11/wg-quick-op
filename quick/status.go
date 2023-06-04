package quick

import (
	"github.com/BaiMeow/wg-quick-op/conf"
	"golang.zx2c4.com/wireguard/wgctrl"
	"time"
)

func IsConnected(iface string) (bool, error) {
	c, err := wgctrl.New()
	if err != nil {
		return false, err
	}
	device, err := c.Device(iface)
	if err != nil {
		return false, err
	}

	for _, peer := range device.Peers {
		if time.Now().Sub(peer.LastHandshakeTime) < conf.DDNS.MaxLastHandleShake {
			return true, nil
		}
	}
	return false, nil
}
