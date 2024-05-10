package utils

import (
	"github.com/sirupsen/logrus"
	"os"
	"slices"
	"strings"
	"time"
)

func GoRetry(times int, f func() error) <-chan error {
	done := make(chan error)
	go func() {
		wait := time.Second
		var err error
		for times > 0 {
			err = f()
			if err == nil {
				break
			}
			time.Sleep(wait)
			wait = wait * 2
			times--
		}
		if err != nil {
			done <- err
		}
		close(done)
	}()
	return done
}

func FindIface(only []string, skip []string) []string {
	if only != nil {
		return only
	}

	var ifaceList []string
	entry, err := os.ReadDir("/etc/wireguard")
	if err != nil {
		logrus.WithError(err).Errorln("read dir /etc/wireguard failed when find iface")
		return nil
	}
	for _, v := range entry {
		if !strings.HasSuffix(v.Name(), ".conf") {
			continue
		}
		name := strings.TrimSuffix(v.Name(), ".conf")
		if slices.Index(skip, name) != -1 {
			continue
		}
		ifaceList = append(ifaceList, name)
	}
	return ifaceList
}
