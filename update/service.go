package update

import (
	"fmt"
	"os/exec"
	"strings"
)

func trySyncServiceScripts(newBin string, strict bool) error {
	openwrt := fileExists("/etc/init.d/wg-quick-op")
	systemdUnit := fileExists("/etc/systemd/system/wg-quick-op.service") || fileExists("/lib/systemd/system/wg-quick-op.service")

	if !openwrt && !systemdUnit {
		if strict {
			return fmt.Errorf("no service scripts detected, nothing to sync")
		}
		fmt.Println("no existing service scripts detected, skip syncing")
		return nil
	}

	out, err := exec.Command(newBin, "install").CombinedOutput()
	if err != nil {
		return fmt.Errorf("sync service scripts failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	if commandExists("systemctl") && systemdUnit {
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}

	return nil
}
