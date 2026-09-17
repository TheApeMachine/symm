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
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
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
			epoch := processStartedAt.UnixNano()

			uiTee := ui.NewUITee(ctx, "uiTee", 131072)
			storeTee := hindsight.NewStoreTee(ctx, "storeTee", 131072)

			// Hindsight's record families are Iceberg tables. The object store
			// above keeps only genuine blobs, the model checkpoint chief among
			// them; everything a reader queries lives in the catalog.
			catalog := tables.Open(ctx)

			if catalog == nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[root] catalog initialization failed", nil,
				))
			}

			if err := catalog.Ensure(ctx); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[root] catalog initialization failed", err,
				))
			}

			grid := store.NewGrid[*geometry.Coordinate]()

			public := websocket.New(
				ctx,
				websocket.NewSimulator(),
				false,
				system.Cfg.WebSocket.Endpoints.Public,
				grid,
			)

			private := websocket.New(
				ctx,
				websocket.NewSimulator(),
				true,
				system.Cfg.WebSocket.Endpoints.Private,
				grid,
			)

			futures := websocket.NewFutures(
				ctx,
				system.Cfg.WebSocket.Endpoints.Futures,
				grid,
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

			training := strategy.NewTraining[*geometry.Coordinate](
				ctx, grid, ui.NewTrainingPublisher(uiTee, storeTee, "BTC/USD"),
			)
			grid.OnWrite(training.Wake)

			if err := catalog.RecordRun(ctx, tables.Run{
				Epoch:        epoch,
				StartedAt:    processStartedAt,
				BuildID:      "training",
				ConfigDigest: viper.GetString("system.log.level"),
				Status:       "ACTIVE",
			}); err != nil {
				return errnie.Error(errnie.Err(errnie.IO, "cmd: record training run", err))
			}

			hub := ui.NewHub(ctx, nil, catalog, uiTee)
			hub.Run()

			manifoldSolver := manifold.NewSolver(ctx, api)

			correlationTicker := correlation.NewTicker(ctx, grid, "", "")
			leadlagTicker := leadlag.NewTicker(ctx, grid, "", "")
			liquidityTicker := liquidity.NewTicker(ctx, grid, "")
			sentimentTicker := sentiment.NewTicker(ctx, grid, "")
			pumpdumpTicker := pumpdump.NewTicker(ctx, grid, "")
			cvdTrade := cvd.NewTrade(ctx, grid, "")
			hawkesTrade := hawkes.NewTrade(ctx, grid, "")
			toxicityTrade := toxicity.NewTrade(ctx, grid, "")
			pumpdumpTrade := pumpdump.NewTrade(ctx, grid, "")
			depthflowLevel3 := depthflow.NewLevel3(ctx, grid, "")
			morphologyLevel3 := morphology.NewLevel3(ctx, grid, "")
			toxicityLevel3 := toxicity.NewLevel3(ctx, grid, "")
			pumpdumpLevel3 := pumpdump.NewLevel3(ctx, grid, "")
			derivativesTicker := derivatives.NewTicker(ctx, grid, "")
			derivativesTrade := derivatives.NewTrade(ctx, grid, "")

			categorySolver := category.NewSolver(ctx)
			resonanceSolver := resonance.NewSolver(
				ctx, system.Cfg.Resonance.LearningRate,
			)
			cognitionSolver := cognition.NewSolver(ctx)
			webrtcTee := ui.NewWebRTCTee(ctx, "webrtcTee", 131072)

			workspace := nmruntime.NewWorkspace(
				ctx, "workspace", [][]core.Primitive{
					{
						public,
						private,
						futures,
					},
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

			// Start consumers before opening market ingress. All construction,
			// subscriptions and seeding have completed at this point.
			for _, runsys := range []nmruntime.RuntimeSystem{
				uiTee,
				storeTee,
				webrtcTee,
				hub,
				training,
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
				api,
			} {
				runsys.Transition(nmruntime.READY)
			}

			drainErrors := make(chan error, 1)

			go func() {
				drainErrors <- catalog.Drain(ctx, epoch, storeTee)
			}()

			manifoldSolver.Start()
			training.Start()

			// Every processing and off-ramp owner is ready before ingress opens.
			for _, connection := range private.Connections() {
				connection.Transition(nmruntime.READY)
			}

			for _, transport := range []nmruntime.RuntimeSystem{public, private, futures} {
				transport.Transition(nmruntime.READY)
			}

			return hub.Fluid.Run(webrtcTee)
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
