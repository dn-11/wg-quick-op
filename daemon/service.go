package daemon

import (
	_ "embed"
	"errors"
	"github.com/BaiMeow/wg-quick-op/conf"
	"github.com/BaiMeow/wg-quick-op/quick"
	"github.com/sirupsen/logrus"
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
		}
		if err := quick.Up(cfg, iface, logrus.WithField("iface", iface)); err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to up interface")
		}
	}

	// resolve
	mp := make(map[string]*quick.Config)
	for _, iface := range conf.DDNS.Iface {
		cfg, err := quick.GetConfig(iface)
		if err != nil {
			logrus.WithField("iface", iface).WithError(err).Error("failed to get config")
		}
		mp[iface] = cfg
	}
	t := time.NewTimer(conf.DDNS.Interval)
	for _ = range t.C {
		for _, iface := range conf.DDNS.Iface {
			if err := quick.Sync(mp[iface], iface, logrus.WithField("iface", iface)); err != nil {
				logrus.WithField("iface", iface).WithError(err).Error("sync failed")
			}
		}
	}
}

func AddService() {
	_, err := exec.LookPath("wg-quick-op")
	if err != nil {
		if !errors.Is(err, exec.ErrDot) {
			logrus.WithError(err).Errorln("look up wg-quick-up failed")
		}
		logrus.Warningln("wg-quick-op hasn't been installed to path")

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
