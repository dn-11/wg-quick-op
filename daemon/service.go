package daemon

import (
	_ "embed"
	"errors"
	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/hdu-dn11/wg-quick-op/utils"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
)

const ServicePath = "/etc/init.d/wg-quick-op"

//go:embed wg-quick-op
var ServiceFile []byte

func Serve() {
	if conf.StartOnBoot.Enabled {
		startOnBoot()
	}

	d := newDaemon()
	d.Run()
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
			if err := <-utils.GoRetry(5, func() error {
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

	logrus.Infoln("all interface parsed")
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
