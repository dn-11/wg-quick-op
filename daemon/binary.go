package daemon

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

const installed = "/usr/sbin/wg-quick-op"

// Install copy binary to /usr/sbin/ (openwrt path)
func Install() {
	file, err := exec.LookPath(os.Args[0])
	if err != nil && !errors.Is(err, exec.ErrDot) {
		log.Err(err).Msg("fetch current binary path failed")
		return
	}

	absFile, err := filepath.Abs(file)
	if err != nil {
		log.Err(err).Str("path", absFile).Msg("The absPath failed")
		return
	}
	log.Info().Msgf("current binary: %v", absFile)

	originFp, err := os.Open(absFile)
	if err != nil {
		log.Err(err).Msgf("open current binary failed")
		return
	}
	defer originFp.Close()

	if _, err := os.Stat(installed); err != nil {
		if !os.IsNotExist(err) {
			log.Err(err).Msgf("fetch binary stat failed")
			return
		}
	} else {
		if err := os.RemoveAll(installed); err != nil {
			log.Err(err).Msgf("remove old binary failed")
			return
		}
	}

	fp, err := os.OpenFile(installed, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		log.Err(err).Msgf("write to %v", installed)
		return
	}
	defer fp.Close()
	_, err = io.Copy(fp, originFp)
	if err != nil {
		_ = os.RemoveAll(installed)
		log.Err(err).Msgf("copy binary to %s", installed)
		return
	}
	log.Info().Msgf("installed wg-quick-op to %s", installed)
}

func Uninstall() {
	file, err := exec.LookPath("wg-quick-op")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err) {
			log.Info().Msg("wg-quick-op binary not found in $PATH, nothing to do")
		} else {
			log.Err(err).Msg("find wg-quick-op binary failed")
		}
		return
	}

	if err := os.RemoveAll(file); err != nil {
		log.Err(err).Str("path", file).Msg("remove wg-quick-op binary failed")
		return
	}
	log.Info().Msgf("removed wg-quick-op from %s", file)
}
