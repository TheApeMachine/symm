package cmd

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/toxicity"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/view"
	"github.com/theapemachine/symm/wallet"
)

var rootCmd = &cobra.Command{
	Use:   "symm",
	Short: "Shake Your Money Maker",
	Long:  rootLong,
	Run: func(cmd *cobra.Command, args []string) {
		pool := qpool.NewQ(
			cmd.Context(), 1, runtime.NumCPU()*4, qpool.NewConfig(),
		)

		qpool.SuppressLogging()

		// Discover the full tradable universe (every online pair in the quote
		// currency) so the signals watch the whole market, not a fixed list.
		if universe := market.DiscoverSymbols(
			cmd.Context(), config.System.QuoteCurrency,
		); len(universe) > 0 {
			config.System.Symbols = universe
		}

		tracker := focus.NewSet()

		booter := errnie.Does(func() (*Booter, error) {
			return NewBooter(cmd.Context(), pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		if err := booter.AddSystems(
			pumpdump.NewSignal(cmd.Context(), pool),
			correlation.NewSignal(cmd.Context(), pool),
			depthflow.NewSignal(cmd.Context(), pool),
			hawkes.NewSignal(cmd.Context(), pool),
			leadlag.NewSignal(cmd.Context(), pool),
			liquidity.NewSignal(cmd.Context(), pool),
			sentiment.NewSignal(cmd.Context(), pool),
			fluid.NewSignal(cmd.Context(), pool),
			causal.NewSignal(cmd.Context(), pool),
			cvd.NewSignal(cmd.Context(), pool),
			toxicity.NewToxicity(cmd.Context(), pool),
			exhaust.NewSignal(cmd.Context(), pool),
			trader.NewCrypto(
				cmd.Context(),
				pool,
				wallet.NewWallet(
					wallet.PaperWallet,
					config.System.QuoteCurrency,
					config.System.WalletEUR,
					config.System.TakerFeePct,
				),
				tracker,
			),
			view.NewOHLC(cmd.Context(), pool, tracker),
			view.NewGauges(cmd.Context(), pool),
		); err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		if err := booter.Boot(); err != nil {
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

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

const rootLong = `
S.Y.M.M. - Shake Your Money Maker

Kraken book and trade observers feed microstructure signals into the trader.
Set SYMM_REPLAY_FILE to a captured JSONL fixture for offline dry-run.
Set SYMM_KRAKEN_API_KEY and SYMM_KRAKEN_API_SECRET for live spot orders over WebSocket v2.
`
