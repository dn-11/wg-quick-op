package cmd

import (
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/sirupsen/logrus"
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
			logrus.Errorln("up command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Up(cfg, iface, logrus.WithField("iface", iface))
			if err != nil {
				logrus.WithError(err).Errorln("failed to up interface")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
