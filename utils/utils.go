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
		// 尝试将错误断言为 *exec.ExitError 类型，以获取退出码
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// 命令已执行，但返回了非零退出码
			return string(out), exitErr.ExitCode(), nil
		} else {
			// 发生了更严重的问题，比如命令本身找不到
			// 返回 -1 表示无法获取退出码
			return string(out), -1, err
		}
	}

	// 命令成功执行，退出码为 0
	return string(out), 0, nil
}
