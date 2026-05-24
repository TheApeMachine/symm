package cmd

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/basis"
	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/depthflow"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/exhaust"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/client"
	krmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/rest"
	"github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
	"github.com/theapemachine/symm/leadlag"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/sentiment"
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

		walletType := trader.PaperWallet

		if config.System.KrakenAPIKey != "" && config.System.KrakenAPISecret != "" {
			walletType = trader.CryptoWallet
		}

		wallet := trader.NewWallet(
			walletType,
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
				frames := errnie.Does(func() ([][]byte, error) {
					return replay.LoadFrames(replayFile)
				}).Or(func(err error) {
					errnie.Error(err)
					os.Exit(1)
				}).Value()

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

		symbolWatch := engine.NewSymbolWatch(symbols)

		uiGroup := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		tickGroup := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
		tradeGroup := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
		bookGroup := pool.CreateBroadcastGroup("book", 10*time.Millisecond)
		chartWatch := map[string]struct{}{"BTC/EUR": {}}

		tradesObserver := errnie.Does(func() (*trades.Trades, error) {
			return trades.New(cmd.Context(), publicClient, symbols, func(
				symbol string,
				batchVolume, buyPressure float64,
				updatedAt time.Time,
				batchTicks []krmarket.TradeTick,
			) {
				tradeGroup.Send(&qpool.QValue[any]{
					SenderID: "kraken:trades",
					Value: engine.TradeUpdate{
						Symbol:      symbol,
						BatchVolume: batchVolume,
						BuyPressure: buyPressure,
						UpdatedAt:   updatedAt,
						Ticks:       batchTicks,
					},
				})
			})
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		bookObserver := errnie.Does(func() (*book.Book, error) {
			return book.New(cmd.Context(), publicClient, symbols, func(
				symbol string,
				spreadBPS, imbalance, density, depthSlope float64,
				bids, asks []krmarket.BookLevel,
				updatedAt time.Time,
			) {
				bookGroup.Send(&qpool.QValue[any]{
					SenderID: "kraken:book",
					Value: engine.BookUpdate{
						Symbol:     symbol,
						SpreadBPS:  spreadBPS,
						Imbalance:  imbalance,
						Density:    density,
						DepthSlope: depthSlope,
						BidLevels:  bids,
						AskLevels:  asks,
						UpdatedAt:  updatedAt,
					},
				})
			})
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		tickerObserver := errnie.Does(func() (*ticker.Ticker, error) {
			quoted := make(map[string]struct{}, len(symbols))

			return ticker.New(cmd.Context(), publicClient, symbols, func(
				symbol string,
				last, bid, ask, volumeBase, changePct float64,
				timestamp string,
			) {
				tickGroup.Send(&qpool.QValue[any]{
					SenderID: "kraken:ticker",
					Value: engine.TickUpdate{
						Symbol:     symbol,
						Last:       last,
						VolumeBase: volumeBase,
						ChangePct:  changePct,
						Timestamp:  timestamp,
					},
				})

				if _, charted := chartWatch[symbol]; charted {
					ui.Publish(uiGroup, "price_tick", map[string]any{
						"symbol":         symbol,
						"last":           last,
						"bid":            bid,
						"ask":            ask,
						"change_pct_24h": changePct,
						"at":             timestamp,
					})
				}

				if _, seen := quoted[symbol]; seen {
					return
				}

				quoted[symbol] = struct{}{}
				ui.Publish(uiGroup, "quote_progress", map[string]any{
					"ready": len(quoted),
					"total": len(symbols),
				})
			})
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

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

		marketRelay := errnie.Does(func() (*engine.MarketRelay, error) {
			return engine.NewMarketRelay(cmd.Context(), tickGroup, tradeGroup, bookGroup)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		pumpSignal := errnie.Does(func() (*pumpdump.PumpDump, error) {
			return pumpdump.NewPumpDump(
				cmd.Context(),
				pool,
				tickGroup,
				tradeGroup,
				bookGroup,
				marketRelay,
				pairIndex,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		hawkesSignal := errnie.Does(func() (*hawkes.Hawkes, error) {
			return hawkes.NewHawkes(
				cmd.Context(),
				pool,
				marketRelay,
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
				pool,
				marketRelay,
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
				pool,
				marketRelay,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		depthflowSignal := errnie.Does(func() (*depthflow.DepthFlow, error) {
			return depthflow.NewDepthFlow(
				cmd.Context(),
				pool,
				marketRelay,
				pairIndex,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		leadlagSignal := errnie.Does(func() (*leadlag.LeadLag, error) {
			return leadlag.NewLeadLag(
				cmd.Context(),
				pool,
				marketRelay,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		basisSignal := errnie.Does(func() (*basis.Basis, error) {
			return basis.NewBasis(
				cmd.Context(),
				pool,
				marketRelay,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		sentimentSignal := errnie.Does(func() (*sentiment.Sentiment, error) {
			return sentiment.NewSentiment(
				cmd.Context(),
				pool,
				marketRelay,
				pairIndex,
				symbols,
				symbolWatch,
			)
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		var telemetryHub *ui.Hub

		cryptoTrader := errnie.Does(func() (*trader.Crypto, error) {
			marketQuotes := trader.NewMarketQuotes(tickerObserver, bookObserver)

			exhaustAdvisor := errnie.Does(func() (*exhaust.Exhaust, error) {
				return exhaust.NewExhaust(cmd.Context(), marketRelay, symbolWatch)
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

			crypto := errnie.Does(func() (*trader.Crypto, error) {
				return trader.NewCrypto(
					cmd.Context(),
					pool,
					uiGroup,
					wallet,
					marketQuotes,
					marketRelay,
					pumpSignal,
					hawkesSignal,
					fluidSignal,
					causalSignal,
					depthflowSignal,
					leadlagSignal,
					basisSignal,
					sentimentSignal,
				)
			}).Or(func(err error) {
				errnie.Error(err)
				os.Exit(1)
			}).Value()

			crypto.SetSymbolUniverse(symbols)

			crypto.BindExitAdvisor(exhaustAdvisor)
			crypto.RegisterTicker(exhaustAdvisor)

			if config.System.KrakenAPIKey != "" && config.System.KrakenAPISecret != "" {
				privateClient := errnie.Does(func() (*client.PrivateClient, error) {
					return client.NewPrivateClient(
						cmd.Context(),
						config.System.KrakenAPIKey,
						config.System.KrakenAPISecret,
					)
				}).Or(func(err error) {
					errnie.Error(err)
					os.Exit(1)
				}).Value()

				if err := privateClient.Connect(); err != nil {
					errnie.Error(err)
					os.Exit(1)
				}

				defer privateClient.Close()

				balance := errnie.Does(func() (*rest.Balance, error) {
					return rest.FetchBalance(
						config.System.KrakenAPIKey,
						config.System.KrakenAPISecret,
					)
				}).Or(func(err error) {
					errnie.Error(err)
					os.Exit(1)
				}).Value()

				if quoteBalance, ok := balance.QuoteBalance(config.System.QuoteCurrency); ok {
					wallet.Balance = quoteBalance
				}

				crypto.BindBroker(trader.NewKrakenBroker(
					privateClient,
					pairIndex,
					config.System.TakerFeePct,
				))

				errnie.Info("live Kraken trading enabled")
			}

			return crypto, nil
		}).Or(func(err error) {
			errnie.Error(err)
			os.Exit(1)
		}).Value()

		cryptoTrader.RegisterTicker(marketRelay)
		fluidSignal.BindUI(uiGroup)
		fluidCommands := ui.NewFluidCommands(fluidSignal, uiGroup)

		if strings.TrimSpace(config.System.ReplayFile) == "" && config.System.OHLCEWarmEnabled {
			_, warmErr := engine.StartupWarm(
				cmd.Context(),
				pairIndex,
				symbols,
				[]engine.Signal{
					pumpSignal,
					hawkesSignal,
					fluidSignal,
					causalSignal,
					depthflowSignal,
					leadlagSignal,
					basisSignal,
					sentimentSignal,
				},
			)

			if warmErr != nil {
				errnie.Error(warmErr)
			}

			if warmErr == nil {
				cryptoTrader.CreditWarmPulses(config.System.OHLCEWarmPulseCredit)
				errnie.Info("ohlc startup warm complete")
			}
		}

		cryptoTrader.PrimeDashboard()

		if _, ok := ui.ListenAddr(config.System.UIAddr); ok {
			telemetryHub = errnie.Does(func() (*ui.Hub, error) {
				return ui.NewHub(cmd.Context(), uiGroup, fluidCommands)
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

		if publicClient.ReplayMode() {
			publicClient.StartReplay()
		}

		if err := cryptoTrader.Run(); err != nil {
			errnie.Error(err)
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
