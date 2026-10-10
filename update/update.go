package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Source string

const (
	SourceAuto   Source = "auto"
	SourceMirror Source = "mirror"
	SourceGitHub Source = "github"
)

type Options struct {
	CurrentVersion    string
	Source            string
	CheckOnly         bool
	Force             bool
	NoRestart         bool
	NoSyncService     bool
	SyncServiceStrict bool
	Timeout           time.Duration
}

func Run(ctx context.Context, opts Options) error {
	ctxTimeout := opts.Timeout
	if ctxTimeout <= 0 {
		ctxTimeout = 120 * time.Second
	}

	usedFlag := Source(opts.Source)
	switch usedFlag {
	case SourceAuto, SourceMirror, SourceGitHub:
	default:
		return fmt.Errorf("invalid --source, expected auto|mirror|github")
	}

	rel, used, err := fetchLatestReleaseWithSource(ctxTimeout, usedFlag)
	if err != nil {
		return err
	}
	latest := normalizeVer(rel.TagName)
	cur := normalizeVer(opts.CurrentVersion)

	if opts.CheckOnly {
		fmt.Printf("Latest: v%s, current: v%s, source: %s\n", latest, cur, used)
		return nil
	}

	if !opts.Force && cur != "" && latest != "" && cur == latest {
		fmt.Printf("Already latest: v%s\n", latest)
		return nil
	}

	assetName := targetAssetName()

	assetURL, err := setAssetURL(rel, used, assetName)
	if err != nil {
		return err
	}

	sumName := checksumAssetName(latest)
	sumURL, err := setAssetURL(rel, used, sumName)
	if err != nil {
		return err
	}

	sumBytes, err := downloadToBytes(sumURL, ctxTimeout, 2<<20)
	if err != nil {
		return err
	}
	expected, err := expectedSHAFromChecksumsBytes(sumBytes, assetName)
	if err != nil {
		return err
	}

	target, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path failed: %w", err)
	}
	if t, err := filepath.EvalSymlinks(target); err == nil {
		target = t
	}

	oldPath := filepath.Join(
		filepath.Dir(target),
		"."+filepath.Base(target)+".old",
	)

	fmt.Printf("Updating to v%s...\n", latest)

	if err := os.Rename(target, oldPath); err != nil {
		return fmt.Errorf("backup old binary failed: %w", err)
	}

	rollback := func(stage string, cause error) error {
		if rerr := os.Rename(oldPath, target); rerr != nil {
			return fmt.Errorf("%s: %v ,rollback failed: %w", stage, cause, rerr)
		}
		return fmt.Errorf("%s: %w", stage, cause)
	}

	if err := streamExtractVerifyTarGzToPath(assetURL, target, expected, ctxTimeout); err != nil {
		return rollback("extract new binary failed", err)
	}

	if err := sanityCheckVersion(ctx, target, latest); err != nil {
		return rollback("sanity check failed", err)
	}

	if !opts.NoSyncService {
		if err := trySyncServiceScripts(target, opts.SyncServiceStrict); err != nil {
			if opts.SyncServiceStrict {
				return rollback("sync service scripts failed", err)
			}
			fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		}
	}

	if !opts.NoRestart {
		if err := restartService(); err != nil {
			if rerr := os.Rename(oldPath, target); rerr != nil {
				return fmt.Errorf("restart service failed: %w; binary rollback also failed: %v", err, rerr)
			}
			if sErr := startService(); sErr != nil {
				return fmt.Errorf("restart service failed: %w; rolled back binary but starting previous version failed: %v", err, sErr)
			}
			return fmt.Errorf("update failed: new version could not restart; rolled back to previous version and restored service (original error: %w)", err)
		}
	}
	_ = os.Remove(oldPath)
	fmt.Printf("Update done: v%s\n", latest)
	return nil
}
