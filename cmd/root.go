// Package cmd implements the crystaldolphin CLI using cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"
const logo = "🐬"

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "crystaldolphin",
	Short: logo + " crystaldolphin — Personal AI Assistant",
	Long:  logo + " crystaldolphin — a lightweight personal AI assistant framework",
}

// Execute runs the root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version

	rootCmd.AddCommand(onboardCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(gatewayCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(cronCmd)
	rootCmd.AddCommand(channelsCmd)
	rootCmd.AddCommand(providerCmd)
}
