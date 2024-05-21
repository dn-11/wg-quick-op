package cmd

import (
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "down [interface name]",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error().Msg("up command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Down(cfg, iface, log.With().Str("iface", iface).Logger())
			if err != nil {
				log.Err(err).Msg("failed to up interface")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
