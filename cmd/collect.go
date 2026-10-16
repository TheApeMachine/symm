package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// collectCmd represents the collect command for continuous, decoupled market ingress
var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect raw market data and sensory measurements continuously",
	Long:  collectLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(
			cmd.Context(), os.Interrupt, syscall.SIGTERM,
		)

		defer cancel()
		return NewBooter(ctx, "collect").Run()
	},
}

func init() {
	rootCmd.AddCommand(collectCmd)
}

var collectLong = `
Collect raw market data continuously and write sensory measurements directly into Iceberg.
This headless daemon gathers unbroken trades, Level 3 book events, and 12-channel sensory
workspace measurements for the shared historical archive without UI or trading dependencies.
`
