package cmd

import (
	"github.com/BaiMeow/wg-quick-op/daemon"

	"github.com/spf13/cobra"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "install wg-quick-op to /usr/bin/wg-quick-op",
	Run: func(cmd *cobra.Command, args []string) {
		daemon.Install()
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
