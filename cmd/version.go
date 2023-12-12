package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "-dev"

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of wg-quick-op",
	Long:  `All software has versions. This is wg-quick-op's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("wg-quick-op v%s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
