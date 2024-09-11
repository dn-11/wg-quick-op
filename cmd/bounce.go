package cmd

import (
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// bounceCmd represents the bounce command
var bounceCmd = &cobra.Command{
	Use:   "bounce",
	Short: "down and then up the interface",
	Long:  `down and then up the interface,if the interface is not up, it will up the interface.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error().Msg("bounce command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Down(cfg, iface, log.With().Str("iface", iface).Logger())
			if err != nil {
				log.Err(err).Str("iface", iface).Msg("failed to down interface")
			}
		}
		for iface, cfg := range cfgs {
			err := quick.Up(cfg, iface, log.With().Str("iface", iface).Logger())
			if err != nil {
				log.Err(err).Str("iface", iface).Msg("failed to up interface")
			}
		}
		log.Info().Msg("bounce done")
	},
}

func init() {
	rootCmd.AddCommand(bounceCmd)
}
