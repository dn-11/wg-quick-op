package cmd

import (
	"github.com/BaiMeow/wg-quick-op/daemon"
	"github.com/spf13/cobra"
)

// installCmd represents the install command
var installServiceCmd = &cobra.Command{
	Use:   "install",
	Short: "install wg-quick-op to /etc/init.d/wg-quick-op",
	Run: func(cmd *cobra.Command, args []string) {
		daemon.AddService()
	},
}

func init() {
	serviceCmd.AddCommand(installServiceCmd)
}
