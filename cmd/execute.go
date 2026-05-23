package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

/*
Execute runs the root command with graceful shutdown on SIGINT/SIGTERM.
*/
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd.SetContext(ctx)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
