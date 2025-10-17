package daemon

import (
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"

	"path/filepath"
	"strings"
)

type WireguardWatcher struct {
	UpdateCallback func(name string)
	RemoveCallback func(name string)
}

func (w *WireguardWatcher) Watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error().Msgf("failed to create watcher: %v", err)
	}
	watcher.Add("/etc/wireguard")
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			_, filename := filepath.Split(event.Name)
			if !strings.HasSuffix(filename, ".conf") {
				continue
			}
			name := strings.TrimSuffix(filename, ".conf")
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				log.Info().Msgf("update file: %s", event.Name)
				if w.UpdateCallback != nil {
					w.UpdateCallback(name)
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				log.Info().Msgf("remove file: %s", event.Name)
				if w.RemoveCallback != nil {
					w.RemoveCallback(name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Err(err).Msgf("watcher error")
		}
	}
}
