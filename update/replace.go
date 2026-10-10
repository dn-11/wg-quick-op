package update

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func sanityCheckVersion(ctx context.Context, binPath, latest string) error {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(checkCtx, binPath, "version").CombinedOutput()
	s := strings.TrimSpace(string(out))

	if checkCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("new binary sanity check timed out")
	}
	if err != nil {
		return fmt.Errorf("new binary sanity check failed: %w (output: %s)", err, s)
	}
	if latest != "" && !strings.Contains(s, latest) {
		return fmt.Errorf("new binary version output mismatch (want contains %q, got: %s)", latest, s)
	}

	if isTTY() && s != "" {
		fmt.Printf("New binary: %s\n", s)
	}
	return nil
}
