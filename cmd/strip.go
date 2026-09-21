package cmd

import (
	"fmt"

	"github.com/dn-11/wg-quick-op/quick"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// stripCmd represents the strip command
var stripCmd = &cobra.Command{
	Use:   "strip [interface name or config file]",
	Short: "Outputs a configuration file suitable for use with wg(8)",
	Long: `strip [interface name or config file]
Outputs a configuration file suitable for use with wg(8),
stripping wg-quick specific directives from the [Interface] section.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error().Msg("strip command requires exactly one interface name or configuration file")
			return
		}
		out, err := quick.Strip(args[0])
		if err != nil {
			log.Err(err).Msg("failed to strip interface configuration")
			return
		}
		fmt.Print(out)
	},
}

func init() {
	rootCmd.AddCommand(stripCmd)
}
