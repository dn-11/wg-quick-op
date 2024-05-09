package daemon

import (
	_ "embed"
	"errors"
	"github.com/hdu-dn11/wg-quick-op/lib/dns"
	"os"
	"os/exec"
	"time"

	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/hdu-dn11/wg-quick-op/utils"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const ServicePath = "/etc/init.d/wg-quick-op"

//go:embed wg-quick-op
var ServiceFile []byte

func Serve() {
	if conf.StartOnBoot.Enabled {
		startOnBoot()
	}

	// prepare config
	var cfgs []*ddns
	for _, iface := range utils.FindIface(conf.DDNS.IfaceOnly, conf.DDNS.IfaceSkip) {
		d, err := newDDNS(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to init ddns config")
			continue
		}
		cfgs = append(cfgs, d)
	}

	for {
		time.Sleep(conf.DDNS.Interval)
		for _, iface := range cfgs {
			peers, err := quick.PeerStatus(iface.name)
			if err != nil {
				logrus.WithError(err).WithField("iface", iface.name).Error("failed to get device")
				continue
			}

			sync := false

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
						sync = true
						break
					}
				}
			}

			if !sync {
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
		logrus.Infoln("endpoint re-resolve done")
	}
}

func startOnBoot() {
	for _, iface := range utils.FindIface(conf.StartOnBoot.IfaceOnly, conf.StartOnBoot.IfaceSkip) {
		iface := iface
		cfg, err := quick.GetConfig(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to get config")
			continue
		}
		go func() {
			if err := <-utils.Retry(5, func() error {
				err := quick.Up(cfg, iface, logrus.WithField("iface", iface))
				if err == nil {
					return nil
				}
				if errors.Is(err, os.ErrExist) {
					logrus.WithField("iface", iface).Infoln("interface already up")
					return nil
				}
				return err
			}); err != nil {
				logrus.WithField("iface", iface).WithError(err).Error("failed to up interface")
				return
			}
			logrus.Infof("interface %s up", iface)
		}()
	}

	logrus.Infoln("all interface up")
}

func AddService() {
	_, err := exec.LookPath("wg-quick-op")
	if err != nil {
		if !errors.Is(err, exec.ErrDot) {
			logrus.WithError(err).Errorln("look up wg-quick-up failed")
		}
		logrus.Warningln("wg-quick-op hasn't been installed to path, let's turn to install it")
		Install()
	}
	if _, err := os.Stat(ServicePath); err == nil {
		err := os.Remove(ServicePath)
		if err != nil {
			logrus.Warnf("remove %s failed", ServicePath)
		}
	}
	file, err := os.OpenFile(ServicePath, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		logrus.WithError(err).Fatalf("open %s failed", ServicePath)
	}
	defer file.Close()
	if _, err := file.Write(ServiceFile); err != nil {
		logrus.WithError(err).Fatalf("write %s failed", ServicePath)
	}
	logrus.Infoln("add wg-quick-op to init.d success")
}

func RmService() {
	err := os.Remove(ServicePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logrus.WithError(err).Errorln("delete service failed")
	}
}
