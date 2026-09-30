package cmd

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
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
	"github.com/theapemachine/symm/workbench"
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

			startPprof()

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

			public := network.NewWebsocketClient(ctx)
			public.Open(system.Cfg.WebSocket.Endpoints.Public)

			private := network.NewWebsocketClient(ctx)
			private.Open(system.Cfg.WebSocket.Endpoints.Private)

			futures := network.NewWebsocketClient(ctx)
			futures.Open(system.Cfg.WebSocket.Endpoints.Futures)

			instrument := broker.NewInstrument(public, futures)
			book := broker.NewBook(ctx, spot.NewNormalizer())
			price := broker.NewPrice(ctx, book, private, instrument)
			balance := broker.NewBalance(ctx, private)

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
			trader := strategy.NewTrader(ctx, private, price, balance)

			wh := workbench.New()
			defer wh.Close()

			webrtcTee := ui.NewUITee(
				ctx, "webrtcTee", 1,
				func(measurement *data.Measurement[float64]) bool {
					return types.Filters(measurement)
				},
			)

			training := strategy.NewTraining(ctx, price, trader, catalog, wh, webrtcTee)

			uiTee.Transition(nmruntime.READY)
			training.Transition(nmruntime.BUSY)
			training.Run()

			// webrtcTee was moved above

			hub := ui.NewHub(ctx, trader, catalog, uiTee, webrtcTee)
			hub.SetPositionSource(trader)

			hub.Run()
			hub.Transition(nmruntime.READY)

			codeCommit, buildID, configDigest := resolveRunIdentity()
			errnie.Info("symm: recording active run in catalog...")

			if err := catalog.RecordRun(ctx, tables.Run{
				Epoch:        epoch,
				StartedAt:    processStartedAt,
				CodeCommit:   codeCommit,
				BuildID:      buildID,
				ConfigDigest: configDigest,
				Status:       "ACTIVE",
			}); err != nil {
				return errnie.Error(errnie.Err(errnie.IO, "cmd: record training run", err))
			}

			manifoldSolver := manifold.NewSolver(ctx, public)

			correlationTicker := correlation.NewTicker(ctx)
			leadlagTicker := leadlag.NewTicker(ctx)
			liquidityTicker := liquidity.NewTicker(ctx)
			sentimentTicker := sentiment.NewTicker(ctx)
			pumpdumpTicker := pumpdump.NewTicker(ctx)
			cvdTrade := cvd.NewTrade(ctx)
			hawkesTrade := hawkes.NewTrade(ctx)
			toxicityTrade := toxicity.NewTrade(ctx)
			pumpdumpTrade := pumpdump.NewTrade(ctx)
			depthflowLevel3 := depthflow.NewLevel3(ctx, book)
			morphologyLevel3 := morphology.NewLevel3(ctx, book)
			toxicityLevel3 := toxicity.NewLevel3(ctx, book)
			pumpdumpLevel3 := pumpdump.NewLevel3(ctx, book)
			derivativesTicker := derivatives.NewTicker(ctx)
			derivativesTrade := derivatives.NewTrade(ctx)

			categorySolver := category.NewSolver(ctx)
			resonanceSolver := resonance.NewSolver(
				ctx, system.Cfg.Resonance.LearningRate,
			)

			cognitionSolver := cognition.NewSolver(ctx)
			workspace := nmruntime.NewWorkspace(
				ctx,
				2,
				"workspace",
				[][]nmruntime.Node[*data.Measurement[float64]]{
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
				webrtcTee,
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

			transportErrors := make(chan error, 1)

			go func() {
				transportErrors <- hub.WebRTC.Run()
			}()

			// Every processing and off-ramp owner is ready before ingress opens.

			for _, transport := range []nmruntime.RuntimeSystem{public, private, futures} {
				transport.Transition(nmruntime.READY)
			}

			startIngress := func(client *network.WebsocketClient) {
				go func() {
					for {
						if ctx.Err() != nil {
							return
						}
						buf, err := client.Read()
						if err != nil {
							continue
						}

						var msg struct {
							Channel string `json:"channel"`
						}
						if err := sonic.Unmarshal(buf, &msg); err != nil {
							continue
						}

						switch msg.Channel {
						case "level3":
							l3 := kraken.NewLevel3(buf)
							if l3 != nil && book != nil {
								book.Update(l3)
							}
						case "ticker":
							t := kraken.NewTicker(buf)
							if t != nil && t.IsSuccess() {
								for _, td := range t.Data {
									m := data.NewMeasurement("websocket", map[string]data.Metric[float64]{
										"bid": {Raw: td.Bid.Float64()},
										"ask": {Raw: td.Ask.Float64()},
									})
									m.Label = td.Symbol
									workspace.Step(m)
								}
							}
						case "trade":
							t := kraken.NewTrade(buf)
							if t != nil && t.IsSuccess() {
								for _, td := range t.Data {
									m := data.NewMeasurement("websocket", map[string]data.Metric[float64]{
										"price":  {Raw: td.Price.Float64()},
										"volume": {Raw: td.Qty},
									})
									m.Label = td.Symbol
									workspace.Step(m)
								}
							}
						}
					}
				}()
			}

			startIngress(public)
			startIngress(private)
			startIngress(futures)

			for ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-transportErrors:
					return errnie.Error(errnie.Err(errnie.IO, "symm: WebRTC publisher stopped", err))
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

func resolveRunIdentity() (codeCommit string, buildID string, configDigest string) {
	buildID = "training"
	if info, ok := debug.ReadBuildInfo(); ok {
		var vcsRev, vcsMod string
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				vcsRev = setting.Value
			}
			if setting.Key == "vcs.modified" {
				vcsMod = setting.Value
			}
		}
		if vcsRev != "" {
			codeCommit = vcsRev

			if vcsMod == "true" {
				codeCommit += "-dirty"
			}
		}

		if codeCommit == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			codeCommit = info.Main.Version
		}
	}

	settings := viper.AllSettings()
	raw, err := json.Marshal(settings)
	if err == nil {
		hash := sha256.Sum256(raw)
		configDigest = hex.EncodeToString(hash[:])
	}

	return codeCommit, buildID, configDigest
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
