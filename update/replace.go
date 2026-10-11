package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// replacer owns the in-place binary swap: it resolves the running executable,
// backs it aside before the new version is written, and restores it if any
// later stage fails. The stored ctx is only consulted to decide whether an
// abort was user-initiated (so the right notice is printed); the renames
// themselves are synchronous syscalls and do not take a ctx.
type replacer struct {
	ctx     context.Context
	target  string
	oldPath string
}

func newReplacer(ctx context.Context) (*replacer, error) {
	target, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path failed: %w", err)
	}
	if t, err := filepath.EvalSymlinks(target); err == nil {
		target = t
	}
	oldPath := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".old")
	return &replacer{ctx: ctx, target: target, oldPath: oldPath}, nil
}

// backup moves the current binary aside so the new one can be written in place.
func (r *replacer) backup() error {
	if err := os.Rename(r.target, r.oldPath); err != nil {
		return fmt.Errorf("backup binary failed: %w", err)
	}
	return nil
}

// restore is the rollback primitive: rename oldPath back over target.
// It reports whether the abort looked user-initiated and returns the rename
// error unwrapped so callers with multi-branch recovery (e.g. restart failure)
// can compose their own messages.
func (r *replacer) restore() error {
	if r.ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "interrupted, rolling back...")
	}
	return os.Rename(r.oldPath, r.target)
}

// rollback restores the previous binary and wraps the outcome into a single
// error describing both the original failure and whether recovery succeeded.
func (r *replacer) rollback(stage string, cause error) error {
	if rerr := r.restore(); rerr != nil {
		return fmt.Errorf("%s: %v; rollback failed: %w", stage, cause, rerr)
	}
	return fmt.Errorf("%s: %w", stage, cause)
}

// cleanup removes the backup after a successful update.
func (r *replacer) cleanup() {
	_ = os.Remove(r.oldPath)
}

func sanityCheckVersion(ctx context.Context, binPath, latest string) error {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(checkCtx, binPath, "version").CombinedOutput()
	s := strings.TrimSpace(string(out))

	if checkCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("new binary timed out")
	}
	if err != nil {
		return fmt.Errorf("new binary exited with error: %w (output: %s)", err, s)
	}
	if latest != "" && !strings.Contains(s, latest) {
		return fmt.Errorf("version output mismatch (want contains %q, got: %s)", latest, s)
	}

	if isTTY() && s != "" {
		fmt.Printf("New binary: %s\n", s)
	}
	return nil
}
