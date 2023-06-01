package cmd

import (
	"github.com/BaiMeow/wg-quick-op/quick"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "down [interface name]",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			logrus.Errorln("up command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Down(cfg, iface, logrus.WithField("iface", iface))
			if err != nil {
				logrus.WithError(err).Errorln("failed to up interface")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
