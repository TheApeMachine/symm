package cmd

import (
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/correlation"
	"github.com/theapemachine/symm/depthflow"
	"github.com/theapemachine/symm/exhaust"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/leadlag"
	"github.com/theapemachine/symm/liquidity"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/sentiment"
	"github.com/theapemachine/symm/trader"
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

		booter := errnie.Does(func() (*Booter, error) {
			return NewBooter(cmd.Context(), pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		predictions := price.NewPrediction(cmd.Context(), pool)

		if err := booter.AddSystems(
			client.NewPublicClient(cmd.Context(), pool, core.KRAKEN_WS_URL),
			pumpdump.NewPumpDump(cmd.Context(), pool),
			correlation.NewSignal(cmd.Context(), pool),
			depthflow.NewDepthFlow(cmd.Context(), pool),
			hawkes.NewHawkes(cmd.Context(), pool),
			leadlag.NewLeadLag(cmd.Context(), pool),
			liquidity.NewLiquidity(cmd.Context(), pool),
			sentiment.NewSentiment(cmd.Context(), pool),
			fluid.NewFluid(cmd.Context(), pool),
			causal.NewCausal(cmd.Context(), pool),
			exhaust.NewExhaust(cmd.Context(), pool),
			predictions,
			trader.NewCrypto(
				cmd.Context(),
				pool,
				wallet.NewWallet(
					wallet.PaperWallet,
					config.System.QuoteCurrency,
					config.System.WalletEUR,
					config.System.TakerFeePct,
				), predictions,
			),
		); err != nil {
			errnie.Error(err)
			os.Exit(1)
		}

		if config.System.KrakenAPIKey != "" && config.System.KrakenAPISecret != "" {
			if err := booter.AddSystems(client.NewPrivateClient(
				cmd.Context(),
				pool,
				core.KRAKEN_WS_AUTH_URL,
				config.System.KrakenAPIKey,
				config.System.KrakenAPISecret,
			)); err != nil {
				errnie.Error(err)
				os.Exit(1)
			}
		}

		if err := booter.Boot(); err != nil {
			errnie.Error(err)
			os.Exit(1)
		}
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
