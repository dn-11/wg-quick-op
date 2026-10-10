package cmd

import (
	"time"

	"github.com/dn-11/wg-quick-op/update"
	"github.com/spf13/cobra"
)

var (
	updateSourceFlag        string
	updateCheckOnly         bool
	updateForce             bool
	updateNoRestart         bool
	updateTimeout           time.Duration
	noUpdateSyncService     bool
	updateSyncServiceStrict bool
)

var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "Self update wg-quick-op from GitHub Releases",
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
		return update.Run(c.Context(), update.Options{
			CurrentVersion:    version,
			Source:            updateSourceFlag,
			CheckOnly:         updateCheckOnly,
			Force:             updateForce,
			NoRestart:         updateNoRestart,
			NoSyncService:     noUpdateSyncService,
			SyncServiceStrict: updateSyncServiceStrict,
			Timeout:           updateTimeout,
		})
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only check latest version, do not update")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already latest")
	updateCmd.Flags().BoolVar(&updateNoRestart, "no-restart", false, "Do not restart service after updating")
	updateCmd.Flags().DurationVar(&updateTimeout, "timeout", 600*time.Second, "Network timeout")
	updateCmd.Flags().StringVar(
		&updateSourceFlag,
		"source",
		string(update.SourceAuto),
		`Update source:
  auto    : mirror -> github
  mirror  : https://mirror.macarons.tech/github/dn-11/wg-quick-op/releases
  github  : https://api.github.com/repos/dn-11/wg-quick-op/releases`,
	)
	updateCmd.Flags().BoolVar(&noUpdateSyncService, "no-sync-service", false, "Do not sync service scripts (systemd unit / OpenWrt init.d) after updating")
	updateCmd.Flags().BoolVar(&updateSyncServiceStrict, "sync-service-strict", false, "Fail update if syncing service scripts fails")

	rootCmd.AddCommand(updateCmd)
}
