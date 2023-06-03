package cmd

import (
	"github.com/BaiMeow/wg-quick-op/conf"
	"github.com/sirupsen/logrus"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "wg-quick-op",
	Short: "wg-quick-op is a tool to manage wireguard interface",
}

var (
	config string
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
		conf.Init(config)
	}
	rootCmd.PersistentFlags().StringVarP(&config, "config", "c", "/etc/wg-quick-op.yaml", "config file (default is $HOME/.wg-quick-op.yaml)")
}
