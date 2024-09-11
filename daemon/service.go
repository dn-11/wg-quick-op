package daemon

import (
	_ "embed"
	"errors"
	"github.com/dn-11/wg-quick-op/conf"
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/dn-11/wg-quick-op/utils"
	"github.com/rs/zerolog/log"

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
			log.Err(err).Str("iface", iface).Msg("failed to get config")
			continue
		}
		go func() {
			if err := <-utils.GoRetry(5, func() error {
				err := quick.Up(cfg, iface, log.With().Str("iface", iface).Logger())
				if err == nil {
					return nil
				}
				if errors.Is(err, os.ErrExist) {
					log.Info().Str("iface", iface).Msg("interface already up")
					return nil
				}
				return err
			}); err != nil {
				log.Err(err).Str("iface", iface).Msg("failed to up interface")
				return
			}
			log.Info().Msgf("interface %s up", iface)
		}()
	}

	log.Info().Msg("all interface parsed")
}

func AddService() {
	_, err := exec.LookPath("wg-quick-op")
	if err != nil {
		if !errors.Is(err, exec.ErrDot) {
			log.Err(err).Msgf("look up wg-quick-up failed")
		}
		log.Warn().Msg("wg-quick-op hasn't been installed to path, let's turn to install it")
		Install()
	}
	if _, err := os.Stat(ServicePath); err == nil {
		err := os.Remove(ServicePath)
		if err != nil {
			log.Warn().Msgf("remove %s failed", ServicePath)
		}
	}
	file, err := os.OpenFile(ServicePath, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Fatal().Err(err).Msgf("open %s failed", ServicePath)
	}
	defer file.Close()
	if _, err := file.Write(ServiceFile); err != nil {
		log.Fatal().Err(err).Msgf("write %s failed", ServicePath)
	}
	log.Info().Msg("add wg-quick-op to init.d success")
}

func RmService() {
	err := os.Remove(ServicePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Err(err).Msgf("delete service failed")
	}
}
