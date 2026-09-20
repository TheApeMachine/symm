package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique/compiler"
)

var (
	graphPath string

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice, or a toaster.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			errnie.Info("[root] compiling system graph: " + graphPath)
			pipeline, err := compiler.CompileFile[any, any](graphPath, nil, definitions.Default())
			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "[root] compilation failed", err))
			}

			errnie.Info("[root] system ready; running pipeline")
			
			focusChan := make(chan string, 100)
			ctx = context.WithValue(ctx, "focusChan", focusChan)
			
			go pipeline.WriteAny(ctx, nil)

			<-ctx.Done()
			errnie.Info("[root] system terminated cleanly")
			return nil
		},
	}
)

/*
Execute executes the root Cobra command.
*/
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&graphPath,
		"graph",
		"g",
		"signal/definitions/system.json",
		"path to the root JSON graph definition",
	)
}

const rootLong = `
Shake your money maker like somebody's 'bout to pay ya
Don't worry about them haters, keep your nose up in the ayer
You know I got it, if you wanna come get it
Stand next to this money like - ey ey ey

Shake, shake, shake your money maker
Like you were shaking it for some paper

...
`
