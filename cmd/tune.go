package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Optimize perspective trees against a recorded capture file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("tune: optimizer wiring is not registered in this build")
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)
}
