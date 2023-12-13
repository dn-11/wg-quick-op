package cmd

import (
	"github.com/hdu-dn11/wg-quick-op/quick"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// bounceCmd represents the bounce command
var bounceCmd = &cobra.Command{
	Use:   "bounce",
	Short: "down and then up the interface",
	Long:  `down and then up the interface,if the interface is not up, it will up the interface.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			logrus.Errorln("bounce command requires exactly one interface name")
			return
		}
		cfgs := quick.MatchConfig(args[0])
		for iface, cfg := range cfgs {
			err := quick.Down(cfg, iface, logrus.WithField("iface", iface))
			if err != nil {
				logrus.WithError(err).WithField("iface", iface).Errorln("failed to down interface")
			}
		}
		for iface, cfg := range cfgs {
			err := quick.Up(cfg, iface, logrus.WithField("iface", iface))
			if err != nil {
				logrus.WithError(err).WithField("iface", iface).Errorln("failed to up interface")
			}
		}
		logrus.Infoln("bounce done")
	},
}

func init() {
	rootCmd.AddCommand(bounceCmd)

}
