package cmd

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
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
			_, err := pyroscope.Start(pyroscope.Config{
				ApplicationName: "symm.theapemachine.app",
				ServerAddress:   "http://localhost:4040",
				Logger:          nil,
			})

			if err != nil {
				log.Fatalf("error starting pyroscope profiler: %v", err)
			}

			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			errnie.Info(fmt.Sprintf(
				"symm started with %d CPUs", runtime.NumCPU(),
			))

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			startPprof()

			// Everything started here implements *runtime.System, which allows you to pass
			// a variadic amount of closers, and everything passes itself to that. There is
			// thus no need to call a deferred Close method for anything.
			hub := ui.NewHub(ctx)

			// Hindsight's record families are Iceberg tables. The object store
			// above keeps only genuine blobs, the model checkpoint chief among
			// them; everything a reader queries lives in the catalog.
			catalog := tables.Open(cmd.Context())

			if catalog == nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open iceberg catalog",
					nil,
				))
			}

			if err := catalog.Ensure(cmd.Context()); err != nil {
				return err
			}

			// The Hindsight inspection reads (runs / captures / persisted states)
			// are served by the hub over this catalog.
			hub.SetHindsightStore(catalog)

			public := websocket.New(
				ctx,
				websocket.NewSimulator(),
				false,
				system.Cfg.WebSocket.Endpoints.Public,
			)

			private := websocket.New(
				ctx,
				websocket.NewSimulator(),
				true,
				system.Cfg.WebSocket.Endpoints.Private,
			)

			futures := websocket.NewFutures(
				ctx,
				system.Cfg.WebSocket.Endpoints.Futures,
			)

			api := websocket.NewAPI(
				ctx, public, private, futures,
			)

			instrument := broker.NewInstrument(api)
			price := broker.NewPrice(ctx, api, instrument)
			balance := broker.NewBalance(ctx, api)

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

			workspaceRegister := store.NewRegister[*data.Measurement[float64]]()

			tape := strategy.NewTape()

			telemetryTee := nmruntime.NewTee(131072)
			telemetryTee.SetFilter(ui.IsAllowedTelemetry)
			go hub.Drain(telemetryTee.Ring())

			epoch := processStartedAt.UnixNano()

			if err := catalog.RecordRun(ctx, tables.Run{
				Epoch:        epoch,
				StartedAt:    processStartedAt,
				BuildID:      "symm-live",
				ConfigDigest: viper.GetString("system.log.level"),
				Status:       "ACTIVE",
			}); err != nil {
				errnie.Warn(fmt.Sprintf("cmd: record run fact: %v", err))
			}

			storageTee := nmruntime.NewNamedTee("storage.tee", 131072)
			go tables.Drain(ctx, catalog, storageTee.Ring(), epoch)

			if catalog != nil {
				hub.SetHindsightStore(catalog)
			}

			tape.Close()

			training := strategy.NewTraining(
				ctx, tape, instrument, price, balance, api,
			)
			hub.SetTradeStore(training)
			hub.SetExitHandler(training.RequestExit)
			hub.SetLearningSource(training)

			workspace := nmruntime.NewWorkspace(
				ctx,
				"workspace",
				[][]nmruntime.Node[*data.Measurement[float64]]{
					{
						public,
						private,
						futures,
					},
					{
						correlation.NewTicker(ctx),
						leadlag.NewTicker(ctx),
						liquidity.NewTicker(ctx),
						sentiment.NewTicker(ctx),
						pumpdump.NewTicker(ctx),
						cvd.NewTrade(ctx),
						hawkes.NewTrade(ctx),
						toxicity.NewTrade(ctx),
						pumpdump.NewTrade(ctx),
						depthflow.NewLevel3(ctx),
						morphology.NewLevel3(ctx),
						toxicity.NewLevel3(ctx),
						pumpdump.NewLevel3(ctx),
						derivatives.NewTicker(ctx),
						derivatives.NewTrade(ctx),
					},
					{
						category.NewSolver(ctx),
						resonance.NewSolver(
							ctx, system.Cfg.Resonance.LearningRate,
						),
						manifold.NewSolver(ctx, api),
					},
					{
						cognition.NewSolver(ctx),
					},
					{
						training,
					},
					{
						telemetryTee,
						storageTee,
					},
				},
				workspaceRegister,
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

			api.Transition(nmruntime.READY)

			if err := api.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: transport readiness barrier failed",
					err,
				))
			}

			if api.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"symm: required transports did not reach ready",
					nil,
				))
			}

			if workspace.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"symm: workspace did not reach ready",
					nil,
				))
			}

			var totalSteps atomic.Uint64

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					if api.Status() != nmruntime.READY ||
						instrument.Status() != nmruntime.READY ||
						price.Status() != nmruntime.READY {
						if workspace.Status() == nmruntime.READY {
							workspace.Transition(nmruntime.WAITING)
						}

						time.Sleep(10 * time.Millisecond)
						continue
					}

					if workspace.Status() == nmruntime.WAITING {
						workspace.Transition(nmruntime.READY)
					}

					if workspace.Status() != nmruntime.READY {
						time.Sleep(10 * time.Millisecond)
						continue
					}

					if public.Pending() > 0 || private.Pending() > 0 || futures.Pending() > 0 {
						workspace.Step(nil)
						totalSteps.Add(1)
						continue
					}

					time.Sleep(100 * time.Microsecond)
				}
			}()

			return hub.Run()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func startPprof() {
	if !viper.GetBool("system.pprof.enabled") && os.Getenv("SYMM_PPROF") == "" {
		return
	}

	addr := viper.GetString("system.pprof.addr")

	if addr == "" {
		addr = "127.0.0.1:6060"
	}

	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	// Pyroscope owns CPU sampling; its handler coordinates a foreground capture.
	mux.HandleFunc("/debug/pprof/cpu", pprof.Profile)

	go func() {
		errnie.Error(http.ListenAndServe(addr, mux))
	}()
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
		if err := tryRead(cfgFile); err == nil {
			loaded = true
		} else {
			fmt.Fprintf(os.Stderr, "symm: config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}
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
