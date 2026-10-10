package update

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	initdPath          = "/etc/init.d/wg-quick-op"
	systemdUnitPathEtc = "/etc/systemd/system/wg-quick-op.service"
	systemdUnitPathLib = "/lib/systemd/system/wg-quick-op.service"
	systemdServiceName = "wg-quick-op.service"
)

func systemdUnitExists() bool {
	return fileExists(systemdUnitPathEtc) || fileExists(systemdUnitPathLib)
}

func trySyncServiceScripts(ctx context.Context, newBin string, strict bool) error {
	openwrt := fileExists(initdPath)
	systemdUnit := systemdUnitExists()

	if !openwrt && !systemdUnit {
		if strict {
			return fmt.Errorf("no service scripts detected, nothing to sync")
		}
		fmt.Println("no existing service scripts detected, skip syncing")
		return nil
	}

	out, err := exec.CommandContext(ctx, newBin, "install").CombinedOutput()
	if err != nil {
		return fmt.Errorf("sync service scripts failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	if commandExists("systemctl") && systemdUnit {
		_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	}

	return nil
}

// restartService restarts wg-quick-op via whichever init system is present.
// Returns nil when no service is installed.
func restartService(ctx context.Context) error {
	if fileExists(initdPath) {
		fmt.Println("restarting service via init.d...")
		return exec.CommandContext(ctx, initdPath, "restart").Run()
	}
	if systemdUnitExists() {
		fmt.Println("restarting service via systemd...")
		return exec.CommandContext(ctx, "systemctl", "restart", systemdServiceName).Run()
	}
	fmt.Println("no service scripts detected, skip restarting")
	return nil
}

// startService starts wg-quick-op via whichever init system is present.
// Used to bring the previous version back after a failed restart and rollback.
func startService(ctx context.Context) error {
	if fileExists(initdPath) {
		return exec.CommandContext(ctx, initdPath, "start").Run()
	}
	if systemdUnitExists() {
		return exec.CommandContext(ctx, "systemctl", "start", systemdServiceName).Run()
	}
	return nil
}
