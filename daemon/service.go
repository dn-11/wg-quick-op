package daemon

import (
	_ "embed"
	"errors"
	"github.com/BaiMeow/wg-quick-op/conf"
	"github.com/BaiMeow/wg-quick-op/quick"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"net"
	"os"
	"os/exec"
	"time"
)

const ServicePath = "/etc/init.d/wg-quick-op"

//go:embed wg-quick-op
var ServiceFile []byte

func Serve() {
	for _, iface := range conf.Enabled {
		cfg, err := quick.GetConfig(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to get config")
			continue
		}
		if err := quick.Up(cfg, iface, logrus.WithField("iface", iface)); err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to up interface, is it already up? turn to run sync")
			if err := quick.Sync(cfg, iface, logrus.WithField("iface", iface)); err != nil {
				logrus.WithField("iface", iface).WithError(err).Error("sync failed")
				continue
			}
		}
		logrus.Infof("interface %s up", iface)
	}

	logrus.Infoln("all interface up")

	// prepare config
	var cfgs []*ddns
	for _, iface := range conf.DDNS.Iface {
		d, err := newDDNS(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to init ddns config")
		}
		cfgs = append(cfgs, d)
	}

	t := time.NewTicker(conf.DDNS.Interval)
	for range t.C {
		for _, iface := range cfgs {
			peers, err := quick.PeerStatus(iface.name)
			if err != nil {
				logrus.WithError(err).WithField("iface", iface).Error("failed to get device")
				continue
			}

			sync := false

			for _, peer := range peers {
				if peer.Endpoint == nil || peer.Endpoint.IP == nil {
					logrus.WithField("iface", iface.name).WithField("peer", peer.PublicKey).Debugln("peer endpoint is nil, skip it")
					continue
				}
				status := peers[peer.PublicKey]
				if time.Now().Sub(status.LastHandshakeTime) < conf.DDNS.MaxLastHandleShake {
					continue
				}
				logrus.WithField("iface", iface.name).WithField("peer", peer.PublicKey).Debugln("peer handshake timeout, re-resolve endpoint")
				endpoint, ok := iface.unresolvedEndpoints[peer.PublicKey]
				if !ok {
					continue
				}
				addr, err := net.ResolveUDPAddr("", endpoint)
				if err != nil {
					logrus.WithField("iface", iface).WithField("peer", peer.PublicKey).WithError(err).Error("failed to resolve endpoint")
					continue
				}
				for i, v := range iface.cfg.Peers {
					if v.PublicKey == peer.PublicKey && addr.AddrPort() != peer.Endpoint.AddrPort() {
						iface.cfg.Peers[i].Endpoint = addr
						sync = true
						break
					}
				}
			}

			if !sync {
				logrus.WithField("iface", iface).Infoln("same addr, skip")
				continue
			}

			link, err := netlink.LinkByName(iface.name)
			if err != nil {
				logrus.WithField("iface", iface).WithError(err).Error("get link failed")
				continue
			}

			if err := quick.SyncWireguardDevice(iface.cfg, link, logrus.WithField("iface", iface)); err != nil {
				logrus.WithField("iface", iface).WithError(err).Error("sync device failed")
				continue
			}

			logrus.WithField("iface", iface.name).Infoln("re-resolve done")
		}
		logrus.Infoln("endpoint re-resolve done")
	}
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
	file, err := os.OpenFile(ServicePath, os.O_CREATE|os.O_RDWR, 755)
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
