package daemon

import (
	"slices"
	"sync"
	"time"

	"github.com/dn-11/wg-quick-op/conf"
	"github.com/dn-11/wg-quick-op/lib/dns"
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/dn-11/wg-quick-op/utils"
	"github.com/rs/zerolog/log"
	"github.com/vishvananda/netlink"
)

type daemon struct {
	runIfaces     map[string]*ddns
	pendingIfaces []string
	lock          sync.Mutex
}

func newDaemon() *daemon {
	d := &daemon{}
	d.runIfaces = make(map[string]*ddns)
	return d
}

func (d *daemon) Run() {
	// prepare config
	for _, iface := range utils.FindIface(conf.DDNS.IfaceOnly, conf.DDNS.IfaceSkip) {
		log.Info().Str("iface", iface).Msg("find iface, init ddns config")
		ddns, err := newDDNS(iface)
		if err != nil {
			log.Err(err).Str("iface", iface).Msg("failed to init ddns config")
			d.pendingIfaces = append(d.pendingIfaces, iface)
			continue
		}
		d.runIfaces[iface] = ddns
	}

	d.registerWatch()
	go d.updateLoop()

	for {
		time.Sleep(conf.DDNS.Interval)
		d.lock.Lock()
		for _, iface := range d.runIfaces {
			peers, err := quick.PeerStatus(iface.name)
			if err != nil {
				log.Err(err).Str("iface", iface.name).Msg("failed to get device")
				continue
			}

			wgUnLink := false

			for _, peer := range peers {
				endpoint, ok := iface.unresolvedEndpoints[peer.PublicKey]
				if !ok {
					log.Debug().Str("iface", iface.name).Str("peer", peer.PublicKey.String()).Msg("peer endpoint is nil, skip it")
					continue
				}
				if time.Since(peer.LastHandshakeTime) < conf.DDNS.HandleShakeMax {
					log.Debug().Str("iface", iface.name).Str("peer", peer.PublicKey.String()).Msg("peer ok")
					continue
				}
				log.Debug().Str("iface", iface.name).Str("peer", peer.PublicKey.String()).Msg("peer handshake timeout")
				wgUnLink = true
				addr, err := dns.ResolveUDPAddr("", endpoint)
				if err != nil {
					log.Err(err).Str("iface", iface.name).Str("peer", peer.PublicKey.String()).Msg("failed to resolve endpoint")
					continue
				}

				for i, v := range iface.cfg.Peers {
					if v.PublicKey == peer.PublicKey && (peer.Endpoint == nil || !peer.Endpoint.IP.Equal(addr.IP)) {
						iface.cfg.Peers[i].Endpoint = addr
						break
					}
				}
			}

			if wgUnLink && *iface.cfg.ListenPort == 0 {
				log.Debug().Str("iface", iface.name).Msgf("randomize listen port")
			}

			if !wgUnLink {
				log.Debug().Str("iface", iface.name).Msg("no update, skip")
				continue
			}

			link, err := netlink.LinkByName(iface.name)
			if err != nil {
				log.Err(err).Str("iface", iface.name).Msg("get link failed")
				continue
			}

			err = quick.SyncWireguardDevice(iface.cfg, link, log.With().Str("iface", iface.name).Logger())
			if err != nil {
				log.Err(err).Str("iface", iface.name).Msg("sync device failed")
				continue
			}

			log.Info().Str("iface", iface.name).Msg("re-resolve done")
		}
		d.lock.Unlock()
		log.Info().Msg("endpoint re-resolve done")
	}
}

func (d *daemon) registerWatch() {
	go (&WireguardWatcher{
		UpdateCallback: func(name string) {
			if conf.DDNS.IfaceOnly != nil && slices.Index(conf.DDNS.IfaceOnly, name) == -1 {
				return
			}
			if conf.DDNS.IfaceSkip != nil && slices.Index(conf.DDNS.IfaceSkip, name) != -1 {
				return
			}
			log.Info().Str("iface", name).Msg("iface update, add to pending list")
			d.lock.Lock()
			defer d.lock.Unlock()
			if slices.Index(d.pendingIfaces, name) == -1 {
				d.pendingIfaces = append(d.pendingIfaces, name)
			}
		},
		RemoveCallback: func(name string) {
			if conf.DDNS.IfaceOnly != nil && slices.Index(conf.DDNS.IfaceOnly, name) == -1 {
				return
			}
			if conf.DDNS.IfaceSkip != nil && slices.Index(conf.DDNS.IfaceSkip, name) != -1 {
				return
			}
			log.Info().Str("iface", name).Msg("iface remove, remove from run list")
			d.lock.Lock()
			defer d.lock.Unlock()
			delete(d.runIfaces, name)
			d.pendingIfaces = slices.DeleteFunc(d.pendingIfaces, func(i string) bool {
				return i == name
			})
		},
	}).Watch()
}

func (d *daemon) updateLoop() {
	for {
		d.lock.Lock()
		var deleteList []string
		for _, iface := range d.pendingIfaces {
			ddns, err := newDDNS(iface)
			if err != nil {
				log.Err(err).Str("iface", iface).Msg("failed to init ddns config")
				continue
			}
			log.Info().Str("iface", iface).Msg("init success, move to run list")
			d.runIfaces[iface] = ddns
			deleteList = append(deleteList, iface)
		}
		for _, iface := range deleteList {
			d.pendingIfaces = slices.DeleteFunc(d.pendingIfaces, func(i string) bool {
				return i == iface
			})
		}
		d.lock.Unlock()
		time.Sleep(conf.DDNS.Interval * 2)
	}
}
