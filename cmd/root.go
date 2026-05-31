package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
)

var rootCmd = &cobra.Command{
	Use:   "symm",
	Short: "Shake Your Money Maker",
	Long:  rootLong,
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := bootEngine(cmd.Context()); err != nil && cmd.Context().Err() == nil {
			errnie.Error(err)
			os.Exit(1)
		}
	},
}

/*
Execute runs the root command with graceful shutdown on SIGINT/SIGTERM. The
signal handler cancels the root context so every system started by Boot
observes ctx.Done() and runs its Close.
*/
func Execute() {
	startProfileServer()

	ctx, cancel := signalNotifyContext()
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

const rootLong = `
S.Y.M.M. - Shake Your Money Maker

Kraken book and trade observers feed microstructure signals into the trader.
Set SYMM_REPLAY_FILE to a captured JSONL fixture for offline dry-run.
Set SYMM_RECORD_FILE to capture exact Kraken WebSocket and REST payloads.
Set SYMM_CONFIG_FILE or runs/tuned.json to load optimizer settings at startup.
Run "symm tune --replay runs/capture.jsonl" to search tunables concurrently.
Set SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET for live spot orders over WebSocket v2.
`
