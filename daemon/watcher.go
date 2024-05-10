package daemon

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"strings"
)

type WireguardWatcher struct {
	UpdateCallback func(name string)
	RemoveCallback func(name string)
}

func (w *WireguardWatcher) Watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("failed to create watcher: %v", err)
	}
	watcher.Add("/etc/wireguard")
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			name := strings.TrimSuffix(event.Name, ".conf")
			if len(name) == len(event.Name) {
				continue
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				logrus.Info("update file:", event.Name)
				if w.UpdateCallback != nil {
					w.UpdateCallback(name)
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				logrus.Info("remove file:", event.Name)
				if w.RemoveCallback != nil {
					w.RemoveCallback(name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logrus.Error("error:", err)
		}
	}
}
