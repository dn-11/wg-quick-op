package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of wg-quick-op",
	Long:  `All software has versions. This is wg-quick-op's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Wireguard Quick for Openwrt v0.0.5")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
