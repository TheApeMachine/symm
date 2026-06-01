package cmd

import (
	"github.com/spf13/cobra"
)

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Search for the best tunables and tree YAML",
	Long:  tuneLong,
	RunE: func(cmd *cobra.Command, args []string) error {

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuneCmd)
}

const tuneLong = `
Run the optimizer, which searches for the best tunables and tree YAML.
`
