package utils

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	ErrUnrecoverable = errors.New("unrecoverable error")
)

func GoRetry(times int, waitBase time.Duration, f func() error) <-chan error {
	done := make(chan error)
	go func() {
		var err error
		for times > 0 {
			wait := waitBase
			err = f()
			if err == nil {
				break
			}
			if errors.Is(err, ErrUnrecoverable) {
				err = errors.Unwrap(err)
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
		log.Err(err).Msg("read dir /etc/wireguard failed when find iface")
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

func RunCommand(name string, arg ...string) (output string, exitCode int, err error) {
	cmd := exec.Command(name, arg...)
	out, err := cmd.CombinedOutput()

	if err != nil {
		// Try to assert the error as *exec.ExitError to get the exit code
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Command executed but returned a non-zero exit code
			return string(out), exitErr.ExitCode(), nil
		}
		// A more serious problem occurred, such as the command not being found
		// Return -1 to indicate that the exit code could not be obtained
		return string(out), -1, err
	}

	// Command executed successfully, exit code is 0
	return string(out), 0, nil
}
