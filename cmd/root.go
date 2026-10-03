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
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
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
				1,
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

			instrument := broker.NewInstrument(public)
			normalizer := spot.NewNormalizer()
			if err := broker.SeedNormalizer(normalizer); err != nil {
				return err
			}
			book := broker.NewBook(ctx, normalizer)
			price := broker.NewPrice(ctx, book, privateTransport, instrument, normalizer)
			balance := broker.NewBalance(ctx, privateTransport)
			if balance.Cash() != nil {
				price.SetReferenceCash(balance.Cash())
			}

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

			desk := broker.NewDesk(ctx, privateTransport, price)

			if paper, ok := privateTransport.(*broker.Paper); ok {
				paper.OnExecution(desk.Apply)
			}

			training := strategy.NewTraining(
				ctx, data.NewArenaOwner(4096),
				price,
				desk,
				catalog,
				storeTee,
			)

			uiTee.Transition(nmruntime.READY)

			// Start historical detector loop which scans trade tape from catalog
			training.Detect()
			training.Train()

			hub := ui.NewHub(ctx, catalog, uiTee)
			hub.SetCognitionSource(training)

			hub.Run()
			hub.Transition(nmruntime.READY)

			manifoldSolver := manifold.NewSolver(ctx, data.NewArenaOwner(4096), book)
			hub.SetManifoldSource(manifoldSolver)
			book.SetNotify(func(symbol string, _ time.Time) {
				manifoldSolver.Wake(symbol)
			})
			correlationSignal := correlation.NewSignal(ctx, data.NewArenaOwner(4096))
			cvdSignal := cvd.NewSignal(ctx, data.NewArenaOwner(4096))
			depthflowSignal := depthflow.NewSignal(ctx, data.NewArenaOwner(4096), book)
			hawkesSignal := hawkes.NewSignal(ctx, data.NewArenaOwner(4096))
			leadlagSignal := leadlag.NewSignal(ctx, data.NewArenaOwner(4096))
			liquiditySignal := liquidity.NewSignal(ctx, data.NewArenaOwner(4096), book)
			morphologySignal := morphology.NewSignal(ctx, data.NewArenaOwner(4096), book)
			pumpdumpSignal := pumpdump.NewSignal(ctx, data.NewArenaOwner(4096), book)
			sentimentSignal := sentiment.NewSignal(ctx, data.NewArenaOwner(4096))
			toxicitySignal := toxicity.NewSignal(ctx, data.NewArenaOwner(4096), book)
			resonanceSolver := resonance.NewSolver(
				ctx, data.NewArenaOwner(4096), system.Cfg.Resonance.LearningRate,
			)

			workspace := nmruntime.NewWorkspace(
				ctx,
				2,
				"workspace",
				[][]nmruntime.Node{
					{
						correlationSignal,
						cvdSignal,
						depthflowSignal,
						hawkesSignal,
						leadlagSignal,
						liquiditySignal,
						morphologySignal,
						pumpdumpSignal,
						sentimentSignal,
						toxicitySignal,
					},
					{
						resonanceSolver,
						manifoldSolver,
					},
					{
						training,
					},
				},
				uiTee,
				storeTee,
			)

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
				manifoldSolver,
				resonanceSolver,
				correlationSignal,
				cvdSignal,
				depthflowSignal,
				hawkesSignal,
				leadlagSignal,
				liquiditySignal,
				morphologySignal,
				pumpdumpSignal,
				sentimentSignal,
				toxicitySignal,
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
			var tick int64

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
						case "trade":
							t := kraken.NewTrade(buf)

							if t != nil && t.IsSuccess() {
								for _, td := range t.Data {
									price.Update(&td)

									metrics := map[string]data.Metric[float64]{
										"price": {
											Raw:   td.Price.Float64(),
											Exact: &td.Price,
										},
										"qty": {
											Raw: td.Qty,
										},
									}

									m := data.NewMeasurement("spot:trade", metrics)
									m.Epoch = epoch

									tick++
									m.Tick = tick

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
