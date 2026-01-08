package cmd

import (
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync (deprecated)",
	Short: "sync [interface name]",
	Long: `sync [interface name], sync link,address,device and route. Notice that PostUp and PreUp won't run
it may result in address added by PostUp being deleted.'`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error().Msg("up command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0], quick.ParseFull)
		for iface, cfg := range cfgs {
			err := quick.Sync(cfg, iface, log.With().Str("iface", iface).Logger())
			if err != nil {
				log.Err(err).Msg("failed to sync interface")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
