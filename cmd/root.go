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

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/catalog/scan"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/compiler"
	nomagiqueruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/ui"
	"github.com/theapemachine/symm/system"
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

			logLevel := viper.GetString("system.log.level")

			if envLevel := os.Getenv("SYMM_LOG_LEVEL"); envLevel != "" {
				logLevel = envLevel
			}

			if logLevel == "" {
				logLevel = "debug"
			}

			errnie.Apply(&errnie.Config{
				Level: logLevel,
			})

			errnie.Info(fmt.Sprintf(
				"symm started with %d CPUs (log level: %s)", runtime.NumCPU(), logLevel,
			))

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			startPprof()

			// Everything started here implements *runtime.System, which allows you to pass
			// a variadic amount of closers, and everything passes itself to that. There is
			// thus no need to call a deferred Close method for anything.
			epoch := processStartedAt.UnixNano()

			// Hindsight's record families are Iceberg tables. The object store
			// above keeps only genuine blobs, the model checkpoint chief among
			// them; everything a reader queries lives in the catalog.
			errnie.Debug("[root] connecting to Iceberg catalog...")
			catalog := tables.Open(ctx)

			if catalog == nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[root] catalog initialization failed", nil,
				))
			}

			errnie.Debug("[root] ensuring Iceberg catalog tables...")
			if err := catalog.Ensure(ctx); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[root] catalog initialization failed", err,
				))
			}

			errnie.Debug("[root] recording training run...")
			if err := catalog.RecordRun(ctx, tables.Run{
				Epoch:        epoch,
				StartedAt:    processStartedAt,
				BuildID:      "training",
				ConfigDigest: logLevel,
				Status:       "ACTIVE",
			}); err != nil {
				return errnie.Error(errnie.Err(errnie.IO, "cmd: record training run", err))
			}

			errnie.Debug("[root] scanning nomagique primitive tree...")
			schemas, err := scan.Tree("nomagique")

			if err != nil {
				return errnie.Error(err)
			}

			errnie.Debug(fmt.Sprintf("[root] scanned %d schemas, building compiler registry...", len(schemas)))
			reg := compiler.NewRegistry(schemas)

			errnie.Debug("[root] loading system graph definition...")
			systemGraph, err := definitions.Load("system")

			if err != nil {
				return errnie.Error(err)
			}

			errnie.Debug(fmt.Sprintf("[root] system graph loaded (%d nodes), compiling...", len(systemGraph.Nodes)))
			systemPipeline, err := compiler.Compile[any](systemGraph, reg)

			if err != nil {
				return errnie.Error(err)
			}

			errnie.Debug("[root] system pipeline compiled successfully, initializing workspace...")
			workspace := nomagiqueruntime.NewWorkspaceWithPipeline(ctx, "system", nomagique.Number[any](systemPipeline))
			defer workspace.Close()

			// 1. Initialize UI Hub and WebRTC server
			errnie.Debug("[root] initializing UI hub and WebRTC server...")
			hub := ui.NewHub(ctx, nil, catalog)
			ui.NewWebRTC(ctx, hub, nil, nil)

			go func() {
				uiAddr := viper.GetString("ui.addr")

				if uiAddr == "" {
					uiAddr = "127.0.0.1:8765"
				}

				errnie.Info(fmt.Sprintf("[root] UI server listening on %s (/ws, /webrtc/manifold)", uiAddr))

				if err := hub.Run(); err != nil {
					errnie.Error(err)
				}
			}()

			// 2. Forward pipeline evaluations to UI Hub
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case result, ok := <-workspace.Sink():
						if !ok {
							return
						}

						if eval, ok := result.(cognition.Evaluation); ok && hub != nil {
							hub.BroadcastEvaluation(eval)
						}
					}
				}
			}()

			// 3. Discover Kraken Universe for Quote Currency
			publicEndpoint := viper.GetString("system.websocket.endpoints.public")
			if publicEndpoint == "" {
				publicEndpoint = "wss://ws.kraken.com/v2"
			}

			l3Endpoint := viper.GetString("system.websocket.endpoints.level3")
			if l3Endpoint == "" {
				l3Endpoint = "wss://ws-l3.kraken.com/v2"
			}

			quoteCurrency := viper.GetString("market.quote_currency")
			if quoteCurrency == "" {
				quoteCurrency = "USD"
			}

			excludedBases := viper.GetStringSlice("market.instrument.excluded")
			errnie.Info(fmt.Sprintf("[root] discovering %s universe from %s (excluding %d bases)...", quoteCurrency, publicEndpoint, len(excludedBases)))

			symbols, err := transport.DiscoverUniverse(ctx, publicEndpoint, quoteCurrency, excludedBases)
			if err != nil {
				errnie.Warn(fmt.Sprintf("[root] dynamic universe discovery failed: %v", err))
				symbols = viper.GetStringSlice("market.symbols")
				if len(symbols) == 0 {
					symbols = []string{"BTC/USD", "ETH/USD"}
				}
			}

			if len(symbols) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[root] no tradeable symbols found in market universe",
					nil,
				))
			}

			batchSize := viper.GetInt("market.subscribe.batch")
			if batchSize <= 0 {
				batchSize = 200
			}

			paceDuration := viper.GetDuration("market.subscribe.pace")
			if paceDuration <= 0 {
				paceDuration = 1 * time.Second
			}

			errnie.Info(fmt.Sprintf("[root] discovered %d online %s pairs; subscribing in batches of %d (pace: %s)...", len(symbols), quoteCurrency, batchSize, paceDuration))

			// 4. Initialize Paper Trading / Execution Runner
			model := viper.GetString("market.model")
			if model == "paper" {
				errnie.Info("[root] initializing paper trading runner...")
				paperRunner := transport.NewPaper(
					ctx,
					func(balance map[string]any) {
						errnie.Debug(fmt.Sprintf("[paper] balance update: %v", balance))
					},
					func(execution map[string]any) {
						errnie.Info(fmt.Sprintf("[paper] execution update: %v", execution))
					},
				)

				bal, err := paperRunner.Balance()
				if err != nil {
					errnie.Warn(fmt.Sprintf("[root] initial paper balance check: %v", err))
				}

				if bal != nil {
					errnie.Info(fmt.Sprintf("[root] paper trading initial balance: %v", bal["balances"]))
				}

				paperRunner.StartPoll(10 * time.Second)
			}

			// 5. Start Kraken WebSocket Ingress
			errnie.Info(fmt.Sprintf("[root] connecting to Kraken public WebSocket (%s) for %d symbols...", publicEndpoint, len(symbols)))
			go transport.StartWSIngress(
				ctx,
				publicEndpoint,
				[]string{"ticker", "trade", "book"},
				symbols,
				batchSize,
				paceDuration,
				func(tick any) {
					workspace.Next(tick)
				},
			)

			errnie.Info(fmt.Sprintf("[root] connecting to Kraken Level3 WebSocket (%s) for %d symbols...", l3Endpoint, len(symbols)))
			go transport.StartWSIngress(
				ctx,
				l3Endpoint,
				[]string{"level3"},
				symbols,
				batchSize,
				paceDuration,
				func(tick any) {
					workspace.Next(tick)
				},
			)

			errnie.Info("[root] system ready; listening on context")
			<-ctx.Done()
			return nil
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
