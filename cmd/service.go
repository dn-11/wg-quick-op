package cmd

import (
	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/hdu-dn11/wg-quick-op/daemon"

	"github.com/spf13/cobra"
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "run service in backend",
	Long: `run service in backend. 
the service will read config file, according to the config file, it do ddns resolve updating, specific interface upping and so on`,
	Run: func(cmd *cobra.Command, args []string) {
		conf.Init(config)
		daemon.Serve()
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)
}
