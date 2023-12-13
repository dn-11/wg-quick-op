package cmd

import (
	"github.com/BaiMeow/wg-quick-op/daemon"

	"github.com/spf13/cobra"
)

// uninstallCmd represents the uninstall command
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "uninstall wg-quick-op from /usr/sbin/wg-quick-op",
	Run: func(cmd *cobra.Command, args []string) {
		daemon.RmService()
		daemon.Uninstall()
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
