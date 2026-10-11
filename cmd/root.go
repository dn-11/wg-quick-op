package cmd

import (
	"context"
	"os"

	"github.com/dn-11/wg-quick-op/conf"
	"github.com/dn-11/wg-quick-op/lib/dns"
	"github.com/rs/zerolog"

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

func Execute(ctx context.Context) {
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
		}
		conf.Init(config, verbose)
		dns.Init()
	}
	rootCmd.PersistentFlags().StringVarP(&config, "config", "c", "/etc/wg-quick-op.toml", "config file path")
}
