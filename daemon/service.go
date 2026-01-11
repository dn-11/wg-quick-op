package daemon

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/dn-11/wg-quick-op/conf"
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/dn-11/wg-quick-op/utils"
	"github.com/rs/zerolog/log"

	"os"
	"os/exec"
)

const InitdServicePath = "/etc/init.d/wg-quick-op"
const SystemdServicePath = "/etc/systemd/system/wg-quick-op.service"

//go:embed wg-quick-op
var InitdServiceFile []byte

//go:embed wg-quick-op-systemd
var SystemdServiceFile []byte

var isSystemd bool

func init() {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		isSystemd = true
	}
}

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
			if err := <-utils.GoRetryCtx(context.Background(), 5, time.Second, func(_ context.Context) error {
				err := quick.Up(cfg, iface, log.With().Str("iface", iface).Logger())
				if err == nil {
					return nil
				}
				if errors.Is(err, os.ErrExist) {
					log.Info().Str("iface", iface).Msg("interface already up")
					return nil
				}
				log.Err(err).Str("iface", iface).Msg("failed to up interface, retrying...")
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
	if isSystemd {
		addSystemdService()
	} else {
		addInitdService()
	}
}

func addSystemdService() {
	log.Info().Msg("systemd detected. Installing systemd service...")
	err := os.WriteFile(SystemdServicePath, SystemdServiceFile, 0644)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to write systemd service file to %s", SystemdServicePath)
	}

	output, exitCode, err := utils.RunCommand("systemctl", "daemon-reload")
	if err != nil || exitCode != 0 {
		log.Error().Err(err).Int("exitCode", exitCode).Str("output", output).Msg("Failed to run 'systemctl daemon-reload'. Please run it manually.")
	} else {
		log.Info().Msg("'systemctl daemon-reload' executed successfully.")
	}

	log.Info().Msg("successfully installed systemd service.")
}

func addInitdService() {
	log.Info().Msg("init.d detected. Installing init.d service...")
	if _, err := os.Stat(InitdServicePath); err == nil {
		err := os.Remove(InitdServicePath)
		if err != nil {
			log.Warn().Msgf("remove %s failed", InitdServicePath)
		}
	}
	file, err := os.OpenFile(InitdServicePath, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Fatal().Err(err).Msgf("open %s failed", InitdServicePath)
	}
	defer file.Close()
	if _, err := file.Write(InitdServiceFile); err != nil {
		log.Fatal().Err(err).Msgf("write %s failed", InitdServicePath)
	}
	log.Info().Msg("add wg-quick-op to init.d success")
}

func RmService() {
	if isSystemd {
		rmSystemdService()
	} else {
		rmInitdService()
	}
}

func rmSystemdService() {
	log.Info().Msg("Removing systemd service...")

	// systemctl disable usually returns exit code 1 when the service does not exist
	output, exitCode, err := utils.RunCommand("systemctl", "disable", "wg-quick-op.service")
	if err != nil {
		log.Error().Err(err).Str("output", output).Msg("Failed to execute 'systemctl disable wg-quick-op.service'")
	} else if exitCode != 0 && exitCode != 1 {
		log.Warn().Int("exitCode", exitCode).Str("output", output).Msg("'systemctl disable wg-quick-op.service' finished with an unexpected exit code")
	} else {
		log.Info().Msg("'systemctl disable wg-quick-op.service' executed successfully")
	}

	// systemctl stop usually returns exit code 5 when the service does not exist
	output, exitCode, err = utils.RunCommand("systemctl", "stop", "wg-quick-op.service")
	if err != nil {
		log.Error().Err(err).Str("output", output).Msg("Failed to execute 'systemctl stop wg-quick-op.service'")
	} else if exitCode != 0 && exitCode != 5 {
		log.Warn().Int("exitCode", exitCode).Str("output", output).Msg("'systemctl stop wg-quick-op.service' finished with an unexpected exit code")
	} else {
		log.Info().Msg("'systemctl stop wg-quick-op.service' executed successfully")
	}

	if err := os.Remove(SystemdServicePath); err != nil {
		if os.IsNotExist(err) {
			log.Info().Msgf("service file %s not found, nothing to remove.", SystemdServicePath)
		} else {
			log.Err(err).Msgf("failed to delete service file %s", SystemdServicePath)
		}
	} else {
		log.Info().Msgf("removed service file %s", SystemdServicePath)
	}

	output, exitCode, err = utils.RunCommand("systemctl", "daemon-reload")
	if err != nil || exitCode != 0 {
		log.Error().Err(err).Int("exitCode", exitCode).Str("output", output).Msg("Failed to run 'systemctl daemon-reload'. Please run it manually.")
	} else {
		log.Info().Msg("'systemctl daemon-reload' executed successfully.")
	}
}

func rmInitdService() {
	log.Info().Msg("Removing init.d service...")

	output, exitCode, err := utils.RunCommand(InitdServicePath, "stop")
	if err != nil {
		log.Info().Err(err).Msgf("could not execute stop command, the script '%s' may have already been removed.", InitdServicePath)
	} else if exitCode != 0 {
		log.Warn().Int("exitCode", exitCode).Str("output", output).Msg("'stop' command finished with an unexpected exit code")
	} else {
		log.Info().Msg("service stopped successfully via init.d script")
	}

	// On some systems, you may also need to run 'update-rc.d -f wg-quick-op remove' to clean up startup links
	if err := os.Remove(InitdServicePath); err != nil {
		if os.IsNotExist(err) {
			log.Info().Msgf("service file %s not found, nothing to remove.", InitdServicePath)
			return
		}
		log.Err(err).Msgf("failed to delete service file %s", InitdServicePath)
		return
	} else {
		log.Info().Msgf("removed service file %s", InitdServicePath)
	}
}
