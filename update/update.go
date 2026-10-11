package update

import (
	"context"
	"fmt"
	"os"
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
	Source            Source
	CheckOnly         bool
	Force             bool
	NoRestart         bool
	NoSyncService     bool
	SyncServiceStrict bool
	Timeout           time.Duration
}

func Run(ctx context.Context, opts Options) error {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rel, used, err := fetchLatestReleaseWithSource(ctx, opts.Source)
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

	sumBytes, err := downloadToBytes(ctx, sumURL, 2<<20)
	if err != nil {
		return err
	}
	expected, err := expectedSHAFromChecksumsBytes(sumBytes, assetName)
	if err != nil {
		return err
	}

	rp, err := newReplacer(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Updating to v%s...\n", latest)

	if err := rp.backup(); err != nil {
		return err
	}

	if err := streamExtractVerifyTarGzToPath(ctx, assetURL, rp.target, expected); err != nil {
		return rp.rollback("extract new binary failed", err)
	}

	if err := sanityCheckVersion(ctx, rp.target, latest); err != nil {
		return rp.rollback("sanity check failed", err)
	}

	if !opts.NoSyncService {
		if err := trySyncServiceScripts(ctx, rp.target, opts.SyncServiceStrict); err != nil {
			if opts.SyncServiceStrict {
				return rp.rollback("sync service scripts failed", err)
			}
			fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		}
	}

	if !opts.NoRestart {
		if err := restartService(ctx); err != nil {
			// Rollback must survive the parent ctx being canceled (Ctrl+C or
			// overall timeout); otherwise the recovery exec would be killed
			// before it can bring the old version back up.
			rbCtx := context.WithoutCancel(ctx)
			if rerr := rp.restore(); rerr != nil {
				return fmt.Errorf("restart service failed: %w; binary rollback also failed: %v", err, rerr)
			}
			if sErr := startService(rbCtx); sErr != nil {
				return fmt.Errorf("restart service failed: %w; rolled back binary but starting previous version failed: %v", err, sErr)
			}
			return fmt.Errorf("update failed: new version could not restart; rolled back to previous version and restored service (original error: %w)", err)
		}
	}
	rp.cleanup()
	fmt.Printf("Update done: v%s\n", latest)
	return nil
}
