package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/derivatives"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/morphology"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Embed a mini filesystem into the binary to hold the default config file.
This will be written to the home directory of the user running the service,
which allows a developer to easily override the config file.
*/
//go:embed cfg/config.yml
var embedded embed.FS

var (
	cfgFile string

	// processStartedAt is the process start instant the Hindsight Run identity
	// is anchored to. It is captured once at process start so a run's identity
	// never shifts as the process runs.
	processStartedAt = time.Now()

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice, or a toaster.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			_, err := pyroscope.Start(pyroscope.Config{
				ApplicationName: "symm.theapemachine.app",
				ServerAddress:   "http://localhost:4040",
				Logger:          nil,
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"[root] error starting pyroscope profiler",
					err,
				))
			}

			errnie.Info(fmt.Sprintf(
				"[root] symm started with %d CPUs", runtime.NumCPU(),
			))

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Everything started here implements *runtime.System, which allows you to pass
			// a variadic amount of closers, and everything passes itself to that. There is
			// thus no need to call a deferred Close method for anything.
			epoch := processStartedAt.UnixNano()

			uiTee := ui.NewUITee(
				ctx, "uiTee",
				4,
				func(measurement *data.Measurement[float64]) bool {
					return types.Filters(measurement)
				},
			)

			// Hindsight's record families are Iceberg tables. The object store
			// above keeps only genuine blobs, the model checkpoint chief among
			// them; everything a reader queries lives in the catalog.
			catalog := tables.Open(ctx)

			if catalog == nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "symm: catalog initialization failed", nil,
				))
			}

			if err := catalog.Ensure(ctx); err != nil {
				return err
			}

			// Register this process in the Runs metadata table so Hindsight can
			// list a capture identity even when the tape is still thin.
			if err := catalog.RecordRun(ctx, tables.Run{
				Epoch:     epoch,
				StartedAt: processStartedAt.UTC(),
				Status:    "running",
			}); err != nil {
				errnie.Warn("[root] failed to record hindsight run: " + err.Error())
			}

			public := network.NewWebsocketClient(ctx)
			if err := public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "symm: public websocket open failed", err,
				))
			}

			futures := network.NewWebsocketClient(ctx)
			if err := futures.Open(system.Cfg.WebSocket.Endpoints.Futures); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "symm: futures websocket open failed", err,
				))
			}

			var (
				privateTransport broker.Transport
				privateWS        *network.WebsocketClient
			)

			if system.Cfg.Market.Model == "paper" {
				privateTransport = broker.NewPaper(ctx)
			}

			if privateTransport == nil {
				privateWS = network.NewWebsocketClient(ctx)
				if err := privateWS.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
					return errnie.Error(errnie.Err(
						errnie.IO, "symm: private websocket open failed", err,
					))
				}
				privateTransport = privateWS
			}

			instrument := broker.NewInstrument(public, futures)
			normalizer := spot.NewNormalizer()
			if err := broker.SeedNormalizer(normalizer); err != nil {
				return err
			}
			book := broker.NewBook(ctx, normalizer)
			price := broker.NewPrice(ctx, book, privateTransport, instrument, normalizer)
			balance := broker.NewBalance(ctx, privateTransport)

			if err := instrument.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: instrument registry failed during construction",
					err,
				))
			}

			if err := price.GetFees(instrument.Symbols()); err != nil {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"[symm] initial fees are not available",
					nil,
				))
			}

			if price.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"[symm] initial price fees are not ready",
					nil,
				))
			}

			if balance.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"[symm] initial balance is not ready",
					nil,
				))
			}

			errnie.Info("symm: initializing training and UI hub...")
			storeTee := hindsight.NewStoreTee(ctx, "storeTee")
			trader := strategy.NewTrader(ctx, privateTransport, price, balance)

			training := strategy.NewTraining(
				ctx, data.NewArenaOwner(4096), epoch, price, trader, catalog, uiTee,
			)

			uiTee.Transition(nmruntime.READY)

			// Start historical training loop which will wait for grid to settle
			training.Run()

			hub := ui.NewHub(ctx, catalog, uiTee)
			hub.SetCognitionSource(training)
			hub.SetPositionSource(trader)

			hub.Run()
			hub.Transition(nmruntime.READY)

			manifoldSolver := manifold.NewSolver(ctx, data.NewArenaOwner(4096), book)
			hub.SetManifoldSource(manifoldSolver)
			book.SetNotify(func(symbol string, _ time.Time) {
				manifoldSolver.Wake(symbol)
			})
			correlationTicker := correlation.NewTicker(ctx, data.NewArenaOwner(4096))
			leadlagTicker := leadlag.NewTicker(ctx, data.NewArenaOwner(4096))
			liquidityTicker := liquidity.NewTicker(ctx, data.NewArenaOwner(4096))
			sentimentTicker := sentiment.NewTicker(ctx, data.NewArenaOwner(4096))
			pumpdumpTicker := pumpdump.NewTicker(ctx, data.NewArenaOwner(4096))
			cvdTrade := cvd.NewTrade(ctx, data.NewArenaOwner(4096))
			hawkesTrade := hawkes.NewTrade(ctx, data.NewArenaOwner(4096))
			toxicityTrade := toxicity.NewTrade(ctx, data.NewArenaOwner(4096))
			pumpdumpTrade := pumpdump.NewTrade(ctx, data.NewArenaOwner(4096))
			depthflowLevel3 := depthflow.NewLevel3(ctx, data.NewArenaOwner(4096), book)
			morphologyLevel3 := morphology.NewLevel3(ctx, data.NewArenaOwner(4096), book)
			toxicityLevel3 := toxicity.NewLevel3(ctx, data.NewArenaOwner(4096), book)
			pumpdumpLevel3 := pumpdump.NewLevel3(ctx, data.NewArenaOwner(4096), book)
			derivativesTicker := derivatives.NewTicker(ctx, data.NewArenaOwner(4096))
			derivativesTrade := derivatives.NewTrade(ctx, data.NewArenaOwner(4096))
			categorySolver := category.NewSolver(ctx, data.NewArenaOwner(4096))
			resonanceSolver := resonance.NewSolver(
				ctx, data.NewArenaOwner(4096), system.Cfg.Resonance.LearningRate,
			)

			cognitionSolver := cognition.NewSolver(ctx, data.NewArenaOwner(4096))
			workspace := nmruntime.NewWorkspace(
				ctx,
				2,
				"workspace",
				[][]nmruntime.Node{
					{
						correlationTicker,
						leadlagTicker,
						liquidityTicker,
						sentimentTicker,
						pumpdumpTicker,
						cvdTrade,
						hawkesTrade,
						toxicityTrade,
						pumpdumpTrade,
						depthflowLevel3,
						morphologyLevel3,
						toxicityLevel3,
						pumpdumpLevel3,
						derivativesTicker,
						derivativesTrade,
					},
					{
						categorySolver,
						resonanceSolver,
						manifoldSolver,
					},
					{
						cognitionSolver,
					},
					{
						training,
					},
				},
				uiTee,
				storeTee,
			)

			// Level3 previously only updated the book — depthflow/morphology/toxicity
			// level3 nodes never received a workspace event. Feed verified touches
			// (real top-of-book from checksum-matched frames, not invented).
			book.SetTouch(func(touches []kraken.Level3Touch) {
				for _, touch := range touches {
					metrics := map[string]data.Metric[float64]{}

					if touch.Bid != nil {
						metrics["bid"] = data.Metric[float64]{
							Raw: touch.Bid.Float64(), Exact: touch.Bid,
						}
					}

					if touch.Ask != nil {
						metrics["ask"] = data.Metric[float64]{
							Raw: touch.Ask.Float64(), Exact: touch.Ask,
						}
					}

					if touch.BidQty != nil {
						metrics["bid_qty"] = data.Metric[float64]{
							Raw: touch.BidQty.Float64(), Exact: touch.BidQty,
						}
					}

					if touch.AskQty != nil {
						metrics["ask_qty"] = data.Metric[float64]{
							Raw: touch.AskQty.Float64(), Exact: touch.AskQty,
						}
					}

					if len(metrics) == 0 || touch.Symbol == "" {
						continue
					}

					m := data.NewMeasurement("spot:level3", metrics)
					m.Epoch = epoch
					m.Label = touch.Symbol
					m.At = touch.Timestamp
					m.SetMetadata("type", "level3_touch")
					m.SetProvenance("ingress_channel", "level3_touch")
					m.SetProvenance("channel", "level3_touch")
					workspace.Step(m)
				}
			})

			// Map spot symbols onto Futures perpetuals before subscribe so the
			// futures socket receives product_ids (not spot names). Soft-fail:
			// a missing catalog leaves spot-only running; derivatives stay dark.
			if err := instrument.LoadFuturesProducts(ctx); err != nil {
				errnie.Warn("[root] futures product map unavailable: " + err.Error())
			}

			// Subscribe and seed while transports remain BUSY. Only a complete
			// instrument universe and restored learner may open the workspace.
			if err := instrument.Subscribe(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: subscribe to instrument universe",
					err,
				))
			}

			if instrument.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"symm: instrument universe is not seeded",
					nil,
				))
			}

			// Live private WS must subscribe executions (+ balances) so fills
			// reach Trader.ApplyExecution — Paper wires OnExecution directly.
			if privateWS != nil {
				auth := kraken.NewAuth()
				token, tokErr := auth.Token()
				if tokErr != nil {
					return errnie.Error(errnie.Err(
						errnie.NotAcceptable,
						"symm: private websocket auth token unavailable",
						tokErr,
					))
				}
				subscribePrivate := func(tok string) error {
					for _, payload := range []any{
						kraken.NewExecutionSubscription(tok),
						kraken.NewBalanceSubscription(tok),
					} {
						msg, err := sonic.Marshal(payload)
						if err != nil {
							return errnie.Error(errnie.Err(
								errnie.IO, "symm: private subscribe marshal failed", err,
							))
						}
						if err := privateWS.Write(msg); err != nil {
							return errnie.Error(errnie.Err(
								errnie.IO, "symm: private subscribe failed", err,
							))
						}
					}
					return nil
				}
				if err := subscribePrivate(token); err != nil {
					return err
				}
				privateWS.OnReconnect(func() error {
					tok, err := auth.Token()
					if err != nil {
						return err
					}
					return subscribePrivate(tok)
				})
			}

			// Start consumers before opening market ingress. All construction,
			// subscriptions and seeding have completed at this point.
			for _, runsys := range []nmruntime.RuntimeSystem{
				uiTee,
				storeTee,
				hub,
				training,
				trader,
				manifoldSolver,
				categorySolver,
				resonanceSolver,
				cognitionSolver,
				correlationTicker,
				leadlagTicker,
				liquidityTicker,
				sentimentTicker,
				pumpdumpTicker,
				cvdTrade,
				hawkesTrade,
				toxicityTrade,
				pumpdumpTrade,
				depthflowLevel3,
				morphologyLevel3,
				toxicityLevel3,
				pumpdumpLevel3,
				derivativesTicker,
				derivativesTrade,
				workspace,
			} {
				runsys.Transition(nmruntime.READY)
			}

			drainErrors := make(chan error, 1)

			go func() {
				drainErrors <- catalog.Drain(ctx, epoch, storeTee)
			}()

			manifoldSolver.Start()

			// Every processing and off-ramp owner is ready before ingress opens.

			startIngress := func(
				client *network.WebsocketClient, name string,
			) {
				go func() {
					errnie.Info(fmt.Sprintf("[root] starting %s ingress", name))

					for {
						select {
						case <-ctx.Done():
							return
						default:
						}

						buf, err := client.Read()

						if err != nil {
							// Soft disconnect/reconnect is expected (1006, reset).
							// Read redials with backoff and warnOnce; do not ERROR-flood here.
							continue
						}

						var msg struct {
							Channel string `json:"channel"`
						}

						if err := sonic.Unmarshal(buf, &msg); err != nil {
							continue
						}

						switch msg.Channel {
						case "executions":
							execution := kraken.NewExecution(buf)
							if execution != nil && trader != nil {
								trader.ApplyExecution(execution)
							}
						case "balances":
							wallet := kraken.NewBalance(buf)
							if wallet != nil && balance != nil {
								balance.UpdateWallet(wallet)
							}
						case "level3":
							l3 := kraken.NewLevel3(buf)
							if l3 != nil && book != nil {
								book.Update(l3)
							}

							if l3 != nil {
								for _, ld := range l3.Data {
									for sideIdx, orders := range [][]kraken.Level3Order{ld.Bids, ld.Asks} {
										side := "bid"
										if sideIdx == 1 {
											side = "ask"
										}

										for _, order := range orders {
											metrics := map[string]data.Metric[float64]{
												"checksum": {Raw: float64(ld.Checksum)},
											}

											if order.LimitPrice != nil {
												metrics["limit_price"] = data.Metric[float64]{
													Raw:   order.LimitPrice.Float64(),
													Exact: order.LimitPrice,
												}
											}

											if order.OrderQty != nil {
												metrics["order_qty"] = data.Metric[float64]{
													Raw:   order.OrderQty.Float64(),
													Exact: order.OrderQty,
												}
											}

											m := data.NewMeasurement("spot:level3", metrics)
											m.Epoch = epoch
											m.Label = ld.Symbol
											m.At = order.Timestamp
											if m.At.IsZero() {
												m.At = ld.Timestamp
											}
											m.SeqIdx = workspace.Sequence()
											m.SetMetadata("type", ld.Type)
											m.SetMetadata("order_id", order.OrderID)
											m.SetMetadata("side", side)
											m.SetMetadata("event", order.Event)
											m.SetMetadata("checksum", fmt.Sprintf("%d", ld.Checksum))
											m.SetProvenance("ingress_channel", "level3")
											m.SetProvenance("channel", "level3")

											storeTee.Push(data.NewPublication(m, nil))
										}
									}
								}
							}
						case "ticker":
							t := kraken.NewTicker(buf)

							if t != nil && t.IsSuccess() {
								for _, td := range t.Data {
									// Venue ticker already carries touch size; liquidity and
									// toxicity gates require bid_qty/ask_qty (not invented).
									metrics := map[string]data.Metric[float64]{
										"volume":  {Raw: td.Volume},
										"bid_qty": {Raw: td.BidQty},
										"ask_qty": {Raw: td.AskQty},
									}

									if td.Bid != nil {
										metrics["bid"] = data.Metric[float64]{
											Raw:   td.Bid.Float64(),
											Exact: td.Bid,
										}
									}

									if td.Ask != nil {
										metrics["ask"] = data.Metric[float64]{
											Raw:   td.Ask.Float64(),
											Exact: td.Ask,
										}
									}

									if td.Last != nil {
										metrics["last"] = data.Metric[float64]{
											Raw:   td.Last.Float64(),
											Exact: td.Last,
										}
									}

									m := data.NewMeasurement("spot:ticker", metrics)
									m.Epoch = epoch
									m.Label = td.Symbol
									m.At = td.Timestamp
									m.SetMetadata("type", "ticker")
									m.SetProvenance("ingress_channel", "ticker")
									m.SetProvenance("channel", "ticker")

									if td.Trades != nil {
										m.SetMetadata("trades", fmt.Sprintf("%d", *td.Trades))
									}

									workspace.Step(m)
								}
							}
						case "trade":
							t := kraken.NewTrade(buf)

							if t != nil && t.IsSuccess() {
								for _, td := range t.Data {
									metrics := map[string]data.Metric[float64]{
										"price": {Raw: td.Price.Float64(), Exact: &td.Price},
										// Pipelines (cvd/hawkes Gates) read qty + Provenance side.
										"qty": {Raw: td.Qty},
									}

									m := data.NewMeasurement("spot:trade", metrics)
									m.Epoch = epoch
									m.Label = td.Symbol
									m.At = td.Timestamp
									m.SetMetadata("type", "trade")
									m.SetMetadata("ord_type", td.OrderType)
									m.SetMetadata("trade_id", fmt.Sprintf("%d", td.TradeID))
									m.SetProvenance("ingress_channel", "trade")
									m.SetProvenance("channel", "trade")
									m.SetProvenance("side", td.Side)

									workspace.Step(m)
								}
							}
						}
					}
				}()
			}

			startIngress(public, "public")

			if privateWS != nil {
				startIngress(privateWS, "private")
			}

			// Futures WS uses feed= (not channel=). Spot ingress cannot parse it.
			go func() {
				errnie.Info("[root] starting futures ingress")
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					buf, err := futures.Read()
					if err != nil {
						continue
					}

					var envelope struct {
						Feed  string `json:"feed"`
						Event string `json:"event"`
					}
					if err := sonic.Unmarshal(buf, &envelope); err != nil {
						continue
					}
					if envelope.Event != "" && envelope.Feed == "" {
						continue
					}

					switch envelope.Feed {
					case "ticker":
						ft := kraken.NewFuturesTicker(buf)
						if ft == nil || ft.Data.ProductID == "" {
							continue
						}
						spot := instrument.SpotForProduct(ft.Data.ProductID)
						if spot == "" {
							continue
						}
						if ft.Data.Last == nil || ft.Data.IndexPrice == nil || ft.Data.MarkPrice == nil {
							continue
						}

						metrics := map[string]data.Metric[float64]{
							"last":          {Raw: ft.Data.Last.Float64(), Exact: ft.Data.Last},
							"index_price":   {Raw: ft.Data.IndexPrice.Float64(), Exact: ft.Data.IndexPrice},
							"mark_price":    {Raw: ft.Data.MarkPrice.Float64(), Exact: ft.Data.MarkPrice},
							"open_interest": {Raw: ft.Data.OpenInterest},
							"volume":        {Raw: ft.Data.Volume},
						}
						if ft.Data.Bid != nil {
							metrics["bid"] = data.Metric[float64]{Raw: ft.Data.Bid.Float64(), Exact: ft.Data.Bid}
						}
						if ft.Data.Ask != nil {
							metrics["ask"] = data.Metric[float64]{Raw: ft.Data.Ask.Float64(), Exact: ft.Data.Ask}
						}

						m := data.NewMeasurement("futures:ticker", metrics)
						m.Epoch = epoch
						m.Label = spot
						m.At = ft.Data.Timestamp
						m.SetMetadata("type", "futures_ticker")
						m.SetProvenance("ingress_channel", "futures_ticker")
						m.SetProvenance("channel", "futures_ticker")
						m.SetProvenance("product_id", ft.Data.ProductID)
						workspace.Step(m)

					case "trade", "trade_snapshot":
						ft := kraken.NewFuturesTrade(buf)
						if ft == nil {
							continue
						}
						for _, td := range ft.Data {
							spot := instrument.SpotForProduct(td.ProductID)
							if spot == "" || td.ProductID == "" {
								continue
							}
							price := td.Price
							metrics := map[string]data.Metric[float64]{
								"price": {Raw: price.Float64(), Exact: &price},
								"qty":   {Raw: td.Qty},
							}
							m := data.NewMeasurement("futures:trade", metrics)
							m.Epoch = epoch
							m.Label = spot
							m.At = td.Timestamp
							m.SetMetadata("type", "futures_trade")
							m.SetProvenance("ingress_channel", "futures_trade")
							m.SetProvenance("channel", "futures_trade")
							m.SetProvenance("product_id", td.ProductID)
							m.SetProvenance("side", td.Side)
							if td.Type != "" {
								m.SetProvenance("type", td.Type)
							}
							workspace.Step(m)
						}
					}
				}
			}()

			instrument.Level3.Range(func(key, value any) bool {
				if client, ok := value.(*network.WebsocketClient); ok {
					startIngress(client, "level3")
				}

				return true
			})

			for ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-drainErrors:
					return errnie.Error(errnie.Err(
						errnie.IO, "symm: catalog drain stopped", err,
					))
				}
			}

			return ctx.Err()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: try cmd/cfg/config.yml, ./config.yml, $HOME/.symm/config.yml, then embedded default)",
	)
}

/*
loadEmbeddedConfig reads the config file baked into the binary, which is the
fallback when no config file is found on disk.
*/
func loadEmbeddedConfig() error {
	viper.SetConfigType("yml")

	cfgReader, err := embedded.Open("cfg/config.yml")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[root] embedded config file not readable",
			err,
		))
	}

	defer cfgReader.Close()

	if err := viper.ReadConfig(cfgReader); err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[root] embedded config file not readable",
			err,
		))
	}

	return nil
}

func initConfig() {
	viper.SetConfigType("yml")

	tryRead := func(path string) error {
		viper.SetConfigFile(path)
		return viper.ReadInConfig()
	}

	loaded := false

	if rootCmd.PersistentFlags().Changed("config") && strings.TrimSpace(cfgFile) != "" {
		if err := tryRead(cfgFile); err != nil {
			fmt.Fprintf(os.Stderr, "symm: config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}

		loaded = true
	}

	if !loaded {
		paths := []string{
			"cmd/cfg/config.yml",
			"config.yml",
		}

		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".symm", "config.yml"))
		}

		for _, p := range paths {
			if err := tryRead(p); err == nil {
				loaded = true
				break
			}
		}
	}

	if !loaded {
		if err := loadEmbeddedConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	// Package initialization ran before Viper loaded the selected file. Build
	// the typed startup generation now, before constructing any live owners.
	system.Cfg = system.NewConfig()

	// Live watching is disabled until an atomic config generation swap exists.
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
