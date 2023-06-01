/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/BaiMeow/wg-quick-op/daemon"

	"github.com/spf13/cobra"
)

// uninstallCmd represents the uninstall command
var uninstallServiceCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "uninstall wg-quick-op service from /etc/init.d/wg-quick-op",
	Run: func(cmd *cobra.Command, args []string) {
		daemon.RmService()
	},
}

func init() {
	serviceCmd.AddCommand(uninstallServiceCmd)
}
