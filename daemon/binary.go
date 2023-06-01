package daemon

import (
	"errors"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const installed = "/usr/sbin/wg-quick-op"

// Install copy binary to /usr/sbin/ (openwrt path)
func Install() {
	file, err := exec.LookPath(os.Args[0])
	if err != nil && !errors.Is(err, exec.ErrDot) {
		logrus.WithError(err).Errorln("fetch current binary path failed")
		return
	}

	absFile, err := filepath.Abs(file)
	if err != nil {
		logrus.WithField("path", absFile).WithError(err).Errorln("The absPath failed")
		return
	}
	logrus.Infof("current binary: %v", absFile)

	originFp, err := os.Open(absFile)
	if err != nil {
		logrus.WithError(err).Errorf("open current binary failed")
		return
	}
	defer originFp.Close()

	if _, err := os.Stat(installed); err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Errorf("fetch binary stat failed")
			return
		}
	} else {
		if err := os.RemoveAll(installed); err != nil {
			logrus.WithError(err).Errorf("remove old binary failed")
			return
		}
	}

	fp, err := os.OpenFile(installed, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		logrus.WithError(err).Errorf("cannot write to %v", installed)
		return
	}
	defer fp.Close()
	_, err = io.Copy(fp, originFp)
	if err != nil {
		_ = os.RemoveAll(installed)
		logrus.Errorf("copy binary to %v failed: %s", installed, err)
		return
	}
	logrus.Infof("installed wg-quick-op")
}

func Uninstall() {
	file, err := exec.LookPath("wg-quick-op")
	if err != nil {
		logrus.WithError(err).Errorln("find wg-quick-op failed")
		return
	}

	if err := os.RemoveAll(file); err != nil {
		logrus.WithField("path", file).WithError(err).Errorln("remove binary failed")
		return
	}
}
