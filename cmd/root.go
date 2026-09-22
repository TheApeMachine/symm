package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	pyroscope "github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
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

			_, err := pyroscope.Start(pyroscope.Config{
				ApplicationName: "symm.theapemachine.app",
				ServerAddress:   "http://localhost:4040",
				Logger:          nil,
			})

			if err != nil {
				log.Fatalf("error starting pyroscope profiler: %v", err)
			}

			errnie.Apply(&errnie.Config{
				Level: "debug",
			})

			errnie.Info(fmt.Sprintf(
				"[root] symm started with %d CPUs", runtime.NumCPU(),
			))

			errnie.Info("[root] compiling system graph: " + graphPath)

			graph, err := compiler.CompileFile(
				graphPath, nil, compiler.DefaultRepository(),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal, "[root] compilation failed", err,
				))
			}

			defer graph.Release()

			errnie.Info("[root] system ready; running graph")
			graph.Start(ctx)

			if err := graph.Flush(context.WithoutCancel(ctx)); err != nil {
				return err
			}
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
		"manifest/system.json",
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
