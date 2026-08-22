package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
ExperimentalCommand runs the normal live stack behind an explicit paper-only
mode flag. Presentation experiments can key off experimental.enabled without
creating a second trading implementation or accidentally enabling real orders.
*/
func ExperimentalCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "experimental",
		Short: "Run the live observability stack in paper-only experimental mode.",
		RunE: func(command *cobra.Command, arguments []string) error {
			viper.Set("experimental.enabled", true)
			viper.Set("trading.model", "paper")
			errnie.Info(fmt.Sprintf(
				"symm experimental enabled: paper model, %d arguments",
				len(arguments),
			))

			if rootCmd.RunE == nil {
				return fmt.Errorf("symm experimental: live command is unavailable")
			}

			return rootCmd.RunE(command, arguments)
		},
	}
}
