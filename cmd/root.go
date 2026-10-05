package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
				func(measurement *data.Measurement) bool {
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

			var privateTransport interface {
				broker.Transport
				Read() ([]byte, error)
			}

			if system.Cfg.Market.Model == "paper" {
				privateTransport = broker.NewPaper(ctx)
			}

			if privateTransport == nil {
				privateWS := network.NewWebsocketClient(ctx)

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

			desk := broker.NewDesk(ctx, privateTransport, price, balance)

			training := strategy.NewTraining(
				ctx, data.NewArenaOwner(4096),
				price,
				desk,
				catalog,
				storeTee,
				epoch,
			)
			training.SetUITee(uiTee)

			uiTee.Transition(nmruntime.READY)

			manifoldSolver := manifold.NewSolver(ctx, data.NewArenaOwner(4096), book)
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

			hub := ui.NewHub(ctx, catalog, uiTee, workspace)
			hub.SetCognitionSource(training)
			hub.SetFragmentsSource(training)
			hub.SetEquitySource(balance)
			hub.SetPositionSource(desk)
			hub.SetExitHandler(func(symbol string) {
				if err := desk.Exit(symbol); err != nil {
					errnie.Error(err)
				}
			})

			hub.Run()
			hub.Transition(nmruntime.READY)

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

			// Start consumers before opening any ingress. All construction and
			// seeding have completed at this point.
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
				client interface{ Read() ([]byte, error) }, name string,
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

						channelNode, err := sonic.Get(buf, "channel")

						if err != nil {
							continue
						}

						channel, err := channelNode.StrictString()

						if err != nil {
							continue
						}

						switch channel {
						case "executions":
							desk.Apply(kraken.NewExecution(buf))
							balance.Invalidate()
						case "balances":
							balance.UpdateWallet(kraken.NewBalance(buf))
							balance.Invalidate()
						case "level3":
							level3Msg := kraken.NewLevel3(buf)
							if level3Msg != nil && book != nil {
								book.Update(level3Msg)
							}

							if level3Msg != nil {
								for _, level3Data := range level3Msg.Data {
									for sideIdx, orders := range [][]kraken.Level3Order{level3Data.Bids, level3Data.Asks} {
										side := "bid"
										if sideIdx == 1 {
											side = "ask"
										}

										checksumStr := strconv.FormatInt(int64(level3Data.Checksum), 10)

										for _, order := range orders {
											measurement := data.NewMeasurement("spot:level3")
											measurement.SetMetric("checksum", data.Metric{Raw: float64(level3Data.Checksum)})

											if order.LimitPrice != nil {
												measurement.SetMetric("limit_price", data.Metric{
													Raw:   kraken.Float64(order.LimitPrice),
													Exact: order.LimitPrice,
												})
											}

											if order.OrderQty != nil {
												measurement.SetMetric("order_qty", data.Metric{
													Raw:   kraken.Float64(order.OrderQty),
													Exact: order.OrderQty,
												})
											}

											measurement.Epoch = epoch
											measurement.Label = level3Data.Symbol
											measurement.At = order.Timestamp
											if measurement.At.IsZero() {
												measurement.At = level3Data.Timestamp
											}
											measurement.SeqIdx = workspace.Sequence()
											measurement.SetMetadata("type", level3Data.Type)
											measurement.SetMetadata("order_id", order.OrderID)
											measurement.SetMetadata("side", side)
											measurement.SetMetadata("event", order.Event)
											measurement.SetMetadata("checksum", checksumStr)
											measurement.SetProvenance("ingress_channel", "level3")
											measurement.SetProvenance("channel", "level3")

											storeTee.Push(data.NewPublication(measurement, nil))
										}
									}
								}
							}
						case "trade":
							tradeMsg := kraken.NewTrade(buf)

							if tradeMsg != nil && tradeMsg.IsSuccess() {
								for _, tradeItem := range tradeMsg.Data {
									price.Update(&tradeItem)

									measurement := data.NewMeasurement("spot:trade")
									measurement.SetMetric("price", data.Metric{
										Raw:   kraken.Float64(&tradeItem.Price),
										Exact: &tradeItem.Price,
									})
									measurement.SetMetric("qty", data.Metric{
										Raw: tradeItem.Qty,
									})

									measurement.Epoch = epoch

									tick++
									measurement.Tick = tick
									measurement.SeqIdx = tick

									measurement.Label = tradeItem.Symbol
									measurement.At = tradeItem.Timestamp

									measurement.SetMetadata("type", "trade")
									measurement.SetMetadata("ord_type", tradeItem.OrderType)
									measurement.SetMetadata("trade_id", strconv.FormatInt(tradeItem.TradeID, 10))
									measurement.SetProvenance("ingress_channel", "trade")
									measurement.SetProvenance("channel", "trade")
									measurement.SetProvenance("side", tradeItem.Side)

									workspace.Step(measurement)
									storeTee.Push(data.NewPublication(measurement, nil))
									balance.Invalidate()
								}
							}
						}
					}
				}()
			}

			// The private reader runs before subscribing: Paper delivers each
			// frame only once the reader takes it, exactly like a live socket.
			startIngress(privateTransport, "private")

			subscribePrivate := func(token string) error {
				for _, payload := range []any{
					kraken.NewExecutionSubscription(token),
					kraken.NewBalanceSubscription(token),
				} {
					msg, err := sonic.Marshal(payload)

					if err != nil {
						return errnie.Error(errnie.Err(
							errnie.IO, "symm: private subscribe marshal failed", err,
						))
					}

					if err := privateTransport.Write(msg); err != nil {
						return errnie.Error(errnie.Err(
							errnie.IO, "symm: private subscribe failed", err,
						))
					}
				}

				return nil
			}

			// Paper accepts the same subscriptions without an auth token.
			var token string

			if privateWS, live := privateTransport.(*network.WebsocketClient); live {
				auth := kraken.NewAuth()
				issued, err := auth.Token()

				if err != nil {
					return errnie.Error(errnie.Err(
						errnie.NotAcceptable,
						"symm: private websocket auth token unavailable",
						err,
					))
				}

				token = issued

				privateWS.OnReconnect(func() error {
					reissued, err := auth.Token()

					if err != nil {
						return errnie.Error(err)
					}

					return subscribePrivate(reissued)
				})
			}

			if err := subscribePrivate(token); err != nil {
				return err
			}

			// The store tee is READY, so historical detections can be stored.
			training.Train()

			startIngress(public, "public")

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
