package cmd

import (
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
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

		wallet := trader.NewWallet(
			trader.PaperWallet,
			config.System.QuoteCurrency,
			config.System.WalletEUR,
			config.System.TakerFeePct,
		)

		publicClient := errnie.Does(func() (*client.PublicClient, error) {
			options := make([]client.PublicClientOption, 0, 3)

			options = append(options, client.OnDisconnect(func(err error) {
				errnie.Error(err)
			}))

			options = append(options, client.OnReconnect(func() {
				errnie.Info("public websocket reconnected")
			}))

			replayFile := strings.TrimSpace(config.System.ReplayFile)
			if replayFile != "" {
				frames, loadErr := replay.LoadFrames(replayFile)
				if loadErr != nil {
					return nil, loadErr
				}

				options = append(options, client.WithReplay(frames, config.System.ReplayPace))
			}

			publicClient := client.NewPublicClient(cmd.Context(), options...)

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
			return market.NewPairs(
				cmd.Context(),
				config.System.QuoteCurrency,
				publicClient,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		symbols := errnie.Does(func() ([]string, error) {
			return pairObserver.Names(cmd.Context())
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		pairIndex := errnie.Does(func() (map[string]asset.Pair, error) {
			assetPairs := errnie.Does(func() ([]asset.Pair, error) {
				return pairObserver.GetAll(cmd.Context())
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

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

		bookObserver := errnie.Does(func() (*book.Book, error) {
			return book.New(cmd.Context(), publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		tradesObserver := errnie.Does(func() (*trades.Trades, error) {
			return trades.New(cmd.Context(), publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		tickerObserver := errnie.Does(func() (*ticker.Ticker, error) {
			return ticker.New(cmd.Context(), publicClient, symbols)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		symbolWatch := engine.NewSymbolWatch(symbols)

		tradesObserver.SetActivityListener(func(symbol string, volume float64) {
			symbolWatch.NoteTrade(symbol, volume)
		})

		bookObserver.SetActivityListener(func(symbol string) {
			symbolWatch.NoteBook(symbol)
		})

		tickerObserver.OnQuote(func(
			symbol string,
			_, _, _, changePct float64,
			_ string,
		) {
			symbolWatch.NoteTicker(symbol, changePct)
		})

		pumpSignal := errnie.Does(func() (*pumpdump.PumpDump, error) {
			return pumpdump.NewPumpDump(
				cmd.Context(),
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		hawkesSignal := errnie.Does(func() (*hawkes.Hawkes, error) {
			return hawkes.NewHawkes(
				cmd.Context(),
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		fluidSignal := errnie.Does(func() (*fluid.Fluid, error) {
			return fluid.NewFluid(
				cmd.Context(),
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		causalSignal := errnie.Does(func() (*causal.Causal, error) {
			return causal.NewCausal(
				cmd.Context(),
				bookObserver,
				tradesObserver,
				tickerObserver,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		var telemetryHub *ui.Hub
		var marketStream *ui.MarketStream

		if _, ok := ui.ListenAddr(config.System.UIAddr); ok {
			telemetryHub = errnie.Does(func() (*ui.Hub, error) {
				return ui.NewHub(cmd.Context(), nil)
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

			go func() {
				if err := telemetryHub.Serve(
					config.System.UIAddr,
				); err != nil && cmd.Context().Err() == nil {
					errnie.Error(err)
				}
			}()
		}

		if telemetryHub != nil {
			marketStream = ui.NewMarketStream(telemetryHub)

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

			telemetryHub.SetFluidDisplayController(fluidSignal)
		}

		cryptoTrader := errnie.Does(func() (*trader.Crypto, error) {
			marketQuotes := trader.NewMarketQuotes(tickerObserver, bookObserver)

			crypto := errnie.Does(func() (*trader.Crypto, error) {
				return trader.NewCrypto(
					cmd.Context(),
					pool,
					wallet,
					marketQuotes,
					pumpSignal,
					hawkesSignal,
					fluidSignal,
					causalSignal,
				)
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

			if marketStream != nil {
				crypto.BindTelemetry(
					marketStream,
					tickerObserver,
					fluidSignal,
					len(symbols),
				)
			}

			return crypto, nil
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

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

const rootLong = `
S.Y.M.M. - Shake Your Money Maker

Kraken book and trade observers feed microstructure signals into the paper trader.
Set SYMM_REPLAY_FILE to a captured JSONL fixture for offline dry-run.
`
