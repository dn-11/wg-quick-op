package quick

import (
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var client *wgctrl.Client

func init() {
	var err error
	client, err = wgctrl.New()
	if err != nil {
		panic(err)
	}
}

func PeerStatus(iface string) (map[wgtypes.Key]*wgtypes.Peer, error) {
	device, err := client.Device(iface)
	if err != nil {
		return nil, err
	}

	peers := make(map[wgtypes.Key]*wgtypes.Peer)
	for _, p := range device.Peers {
		peers[p.PublicKey] = &p
	}
	return peers, nil
}
