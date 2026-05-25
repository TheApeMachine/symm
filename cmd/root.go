package cmd

import (
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/client"
)

var rootCmd = &cobra.Command{
	Use:   "symm",
	Short: "Shake Your Money Maker",
	Long:  rootLong,
	Run: func(cmd *cobra.Command, args []string) {
		pool := qpool.NewQ(
			cmd.Context(), 1, runtime.NumCPU()*2, qpool.NewConfig(),
		)

		qpool.SuppressLogging()

		booter := errnie.Does(func() (*Booter, error) {
			return NewBooter(cmd.Context(), pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		booter.AddSystems(
			client.NewPublicClient(cmd.Context(), pool, "wss://ws.kraken.com/v2/public"),
		)
	},
}

/*
Execute runs the root command with graceful shutdown on SIGINT/SIGTERM.
*/
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type errString string

func (err errString) Error() string { return string(err) }

const rootLong = `
S.Y.M.M. - Shake Your Money Maker

Kraken book and trade observers feed microstructure signals into the trader.
Set SYMM_REPLAY_FILE to a captured JSONL fixture for offline dry-run.
Set SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET for live spot orders over WebSocket v2.
`
