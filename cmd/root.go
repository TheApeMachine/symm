package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/client"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/work"
)

var rootCmd = &cobra.Command{
	Use:   "symm",
	Short: "Shake Your Money Maker — Kraken paper trading",
	Long:  rootLong,
	Run: func(cmd *cobra.Command, args []string) {
		quote, _ := cmd.Flags().GetString("quote")
		walletSize, _ := cmd.Flags().GetFloat64("wallet")
		uiAddr, _ := cmd.Flags().GetString("ui-addr")
		replayFile, _ := cmd.Flags().GetString("replay-file")
		replayPace, _ := cmd.Flags().GetDuration("replay-pace")

		sessionCtx, sessionCancel := context.WithCancel(cmd.Context())
		defer sessionCancel()

		pool := work.NewPool(sessionCtx)
		wallet := trader.NewWallet(
			trader.PaperWallet, quote, walletSize, config.System.TakerFeePct,
		)

		publicClient := errnie.Does(func() (*client.PublicClient, error) {
			options := make([]client.PublicClientOption, 0, 2)

			options = append(options, client.OnDisconnect(func(err error) {
				errnie.Error(err)
				sessionCancel()
			}))

			if replayFile != "" {
				frames, err := replay.LoadFrames(replayFile)

				if err != nil {
					return nil, err
				}

				options = append(options, client.WithReplay(frames, replayPace))
			}

			publicClient := client.NewPublicClient(sessionCtx, options...)

			if err := publicClient.Connect(); err != nil {
				return nil, err
			}

			return publicClient, nil
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		defer publicClient.Close()

		pairObserver := errnie.Does(func() (*market.Pairs, error) {
			return market.NewPairs(sessionCtx, quote, publicClient)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		symbols := errnie.Does(func() ([]string, error) {
			return pairObserver.Names(sessionCtx)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		pairIndex := errnie.Does(func() (map[string]asset.Pair, error) {
			assetPairs, err := pairObserver.GetAll(sessionCtx)

			if err != nil {
				return nil, err
			}

			index := make(map[string]asset.Pair, len(assetPairs))

			for _, pair := range assetPairs {
				name := pair.Wsname

				if name == "" {
					name = pair.Altname
				}

				index[name] = pair
			}

			return index, nil
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		bookObserver := errnie.Does(func() (*kbook.Book, error) {
			return kbook.New(sessionCtx, publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		tradesObserver := errnie.Does(func() (*trades.Trades, error) {
			return trades.New(sessionCtx, publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		tickerObserver := errnie.Does(func() (*kticker.Ticker, error) {
			return kticker.New(sessionCtx, publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		pumpSignal := errnie.Does(func() (*pumpdump.PumpDump, error) {
			return pumpdump.NewPumpDump(
				sessionCtx,
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				config.System.RescoreEvery,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		hawkesSignal := errnie.Does(func() (*hawkes.Hawkes, error) {
			return hawkes.NewHawkes(
				sessionCtx,
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				config.System.RescoreEvery,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		fluidSignal := errnie.Does(func() (*fluid.Fluid, error) {
			return fluid.NewFluid(
				sessionCtx,
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				config.System.RescoreEvery,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		causalSignal := errnie.Does(func() (*causal.Causal, error) {
			return causal.NewCausal(
				sessionCtx,
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				config.System.RescoreEvery,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		var telemetryHub *ui.Hub

		if listenAddr, ok := ui.ListenAddr(uiAddr); ok {
			telemetryHub = errnie.Does(func() (*ui.Hub, error) {
				return ui.NewHub(sessionCtx, nil)
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

			go func() {
				if err := telemetryHub.Serve(listenAddr); err != nil && sessionCtx.Err() == nil {
					errnie.Error(err)
				}
			}()
		}

		if telemetryHub != nil {
			marketStream := ui.NewMarketStream(telemetryHub)

			tickerObserver.OnQuote(func(
				symbol string,
				last, bid, ask, changePct float64,
				timestamp string,
			) {
				marketStream.PriceTick(symbol, last, bid, ask, changePct, timestamp)
			})

			fluidSignal.SetFieldSink(func(snapshot fluid.FieldSnapshot) {
				marketStream.FieldUpdate(snapshot)
			})
		}

		cryptoTrader := errnie.Does(func() (*trader.Crypto, error) {
			crypto, err := trader.NewCrypto(
				sessionCtx,
				pool,
				wallet,
				tickerObserver,
				telemetryHub,
				pumpSignal,
				hawkesSignal,
				fluidSignal,
				causalSignal,
			)

			if err != nil {
				return nil, err
			}

			if telemetryHub != nil {
				telemetryHub.SetBootstrap(crypto.ConnectSnapshot)
			}

			crypto.SetEngineStats(trader.NewEngineStats(
				tickerObserver.ReadyCount,
				func() int { return len(symbols) },
				fluidSignal.SampledCount,
				fluidSignal.WarmingCount,
			))

			return crypto, nil
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		defer cryptoTrader.Close()
		defer pumpSignal.Close()
		defer hawkesSignal.Close()
		defer fluidSignal.Close()
		defer causalSignal.Close()

		if telemetryHub != nil {
			defer telemetryHub.Close()
		}

		if publicClient.ReplayMode() {
			publicClient.StartReplay()
		}

		if err := cryptoTrader.Run(); err != nil {
			errnie.Error(err)
		}
	},
}

type errString string

func (err errString) Error() string { return string(err) }

func init() {
	rootCmd.Flags().Float64("wallet", config.DefaultWalletEUR, "paper wallet size in quote currency")
	rootCmd.Flags().String("quote", config.DefaultQuoteCurrency, "quote currency filter (e.g. EUR)")
	rootCmd.Flags().String("log-level", "info", "trace|debug|info|warn|error")
	rootCmd.Flags().String("log-dir", "runs", "directory for run log files")
	rootCmd.Flags().String("log-file", "", "log file path (default runs/symm-<run_id>.log)")
	rootCmd.Flags().Bool("log-file-active", true, "write logs to --log-file")
	rootCmd.Flags().Bool("log-stdout", true, "mirror logs to stdout")
	rootCmd.Flags().String("ui-addr", config.System.UIAddr, "WebSocket UI telemetry (e.g. :8765); enables ws://host/ws")
	rootCmd.Flags().String("replay-file", "", "newline-delimited Kraken v2 websocket frames for dry-run replay")
	rootCmd.Flags().Duration("replay-pace", 50*time.Millisecond, "delay between replay frames (0 = as fast as possible)")
}

const rootLong = `
S.Y.M.M. - Shake Your Money Maker

Kraken book and trade observers feed microstructure signals into the paper trader.
Use --replay-file with captured websocket JSONL to dry-run without a live feed.
`
