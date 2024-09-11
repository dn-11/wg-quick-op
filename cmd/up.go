package cmd

import (
	"github.com/dn-11/wg-quick-op/quick"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "up [interface name]",
	Long: `up [interface name] 
interface should be defined in /etc/wireguard/<interface name>.conf
regexp in supported, match interface with ^<input>$ by default
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error().Msg("up command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Up(cfg, iface, log.With().Str("iface", iface).Logger())
			if err != nil {
				log.Err(err).Msg("failed to up interface")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
