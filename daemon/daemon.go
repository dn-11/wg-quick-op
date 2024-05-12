package daemon

import (
	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/hdu-dn11/wg-quick-op/lib/dns"
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/hdu-dn11/wg-quick-op/utils"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"slices"
	"sync"
	"time"
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
		logrus.WithField("iface", iface).Infoln("find iface, init ddns config")
		ddns, err := newDDNS(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to init ddns config")
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
				logrus.WithError(err).WithField("iface", iface.name).Error("failed to get device")
				continue
			}

			wgSync := false

			for _, peer := range peers {
				if peer.Endpoint == nil || peer.Endpoint.IP == nil {
					logrus.WithField("iface", iface.name).WithField("peer", peer.PublicKey).Debugln("peer endpoint is nil, skip it")
					continue
				}
				if time.Since(peer.LastHandshakeTime) < conf.DDNS.HandleShakeMax {
					logrus.WithField("iface", iface.name).WithField("peer", peer.PublicKey).Debugln("peer ok")
					continue
				}
				logrus.WithField("iface", iface.name).WithField("peer", peer.PublicKey).Debugln("peer handshake timeout, re-resolve endpoint")
				endpoint, ok := iface.unresolvedEndpoints[peer.PublicKey]
				if !ok {
					continue
				}
				addr, err := dns.ResolveUDPAddr("", endpoint)
				if err != nil {
					logrus.WithField("iface", iface).WithField("peer", peer.PublicKey).WithError(err).Error("failed to resolve endpoint")
					continue
				}

				for i, v := range iface.cfg.Peers {
					if v.PublicKey == peer.PublicKey && !peer.Endpoint.IP.Equal(addr.IP) {
						iface.cfg.Peers[i].Endpoint = addr
						wgSync = true
						break
					}
				}
			}

			if !wgSync {
				logrus.WithField("iface", iface.name).Debugln("same addr, skip")
				continue
			}

			link, err := netlink.LinkByName(iface.name)
			if err != nil {
				logrus.WithField("iface", iface.name).WithError(err).Error("get link failed")
				continue
			}

			if err := quick.SyncWireguardDevice(iface.cfg, link, logrus.WithField("iface", iface.name)); err != nil {
				logrus.WithField("iface", iface.name).WithError(err).Error("sync device failed")
				continue
			}

			logrus.WithField("iface", iface.name).Infoln("re-resolve done")
		}
		d.lock.Unlock()
		logrus.Infoln("endpoint re-resolve done")
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
			logrus.WithField("iface", name).Infoln("iface update, add to pending list")
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
			logrus.WithField("iface", name).Infoln("iface remove, remove from run list")
			d.lock.Lock()
			defer d.lock.Unlock()
			delete(d.runIfaces, name)
			slices.DeleteFunc(d.pendingIfaces, func(i string) bool {
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
				logrus.WithField("iface", iface).WithError(err).Error("failed to init ddns config")
				continue
			}
			logrus.WithField("iface", iface).Infoln("init success, move to run list")
			d.runIfaces[iface] = ddns
			deleteList = append(deleteList, iface)
		}
		for _, iface := range deleteList {
			slices.DeleteFunc(d.pendingIfaces, func(i string) bool {
				return i == iface
			})
		}
		d.lock.Unlock()
		time.Sleep(conf.DDNS.Interval * 2)
	}
}
