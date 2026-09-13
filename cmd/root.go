package cmd

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
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

			hub := ui.NewHub(ctx)
			defer hub.Close()

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

			writer := tables.NewWriter(catalog)

			// The Hindsight inspection reads (runs / captures / persisted states)
			// are served by the hub over this catalog.
			hub.SetHindsightStore(catalog)

			// The Hindsight Run identity distinguishes this process capture
			// session from every other run. It is derived from the process start
			// instant plus a nonce, so two runs can never share an identity, and
			// it carries the config digest actually loaded for this run.
			runID, err := hindsight.NewRunID(processStartedAt)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: derive run identity",
					err,
				))
			}

			rawCapture, err := recording.NewSession(
				ctx, writer, hindsight.RunIdentity{
					StartedAt:      processStartedAt,
					CodeCommit:     buildCodeCommit(),
					BuildID:        buildBuildID(),
					ConfigDigest:   configDigest(),
					SchemaVersions: hindsightSchemaVersions(),
				}.Resolve(runID),
				viper.GetInt("hindsight.capture.batch_size"),
				viper.GetDuration("hindsight.capture.flush_interval"),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[root] failed to initialize raw capture",
					err,
				))
			}

			defer func() {
				if err := rawCapture.Close(); err != nil {
					errnie.Error(err)
				}
			}()

			public := websocket.New(
				ctx,
				websocket.NewSimulator(),
				false,
				system.Cfg.WebSocket.Endpoints.Public,
				rawCapture,
			)

			defer public.Close()

			private := websocket.New(
				ctx,
				websocket.NewSimulator(),
				true,
				system.Cfg.WebSocket.Endpoints.Private,
				rawCapture,
			)

			defer private.Close()

			futures := websocket.NewFutures(
				ctx,
				system.Cfg.WebSocket.Endpoints.Futures,
				rawCapture,
			)

			defer futures.Close()

			api := websocket.NewAPI(
				ctx, public, private, futures,
			)

			defer api.Close()

			instrument := broker.NewInstrument(api)
			price := broker.NewPrice(api, instrument)
			balance := broker.NewBalance(api)

			defer instrument.Close()

			if err := instrument.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: instrument registry failed during construction",
					err,
				))
			}

			// Stateful analytical stages are constructed once and mounted directly
			// in each Workload that produces their inputs. The Workloads themselves
			// remain the complete topology; there is no secondary observation store.
			marketState := types.NewMarketState()

			manifoldSolver := manifold.NewSolver(ctx, api)
			defer manifoldSolver.Close()

			manifoldSolver.SetMarketState(marketState)
			manifoldSolver.SetViewer(hub)
			manifoldSolver.Start()

			if err := price.GetFees(instrument.Symbols()); err != nil {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"[symm] initial fees are not available",
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

			tickerRing := nmruntime.NewWorkload(
				ctx, "ticker",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					public,
				}, {
					correlation.NewTicker(ctx),
					leadlag.NewTicker(ctx),
					liquidity.NewTicker(ctx),
					sentiment.NewTicker(ctx),
					pumpdump.NewTicker(ctx),
				}},
			)

			tradeRing := nmruntime.NewWorkload(
				ctx, "ticker",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					public,
				}, {
					cvd.NewTrade(ctx),
					hawkes.NewTrade(ctx),
					toxicity.NewTrade(ctx),
					pumpdump.NewTrade(ctx),
				}},
			)

			level3Ring := nmruntime.NewWorkload(
				ctx, "level3",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					depthflow.NewLevel3(ctx),
					morphology.NewLevel3(ctx),
					toxicity.NewLevel3(ctx),
					pumpdump.NewLevel3(ctx),
				}},
			)

			futuresTicker := nmruntime.NewWorkload(
				ctx, "futures",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					derivatives.NewTicker(ctx),
				}},
			)

			futuresTrade := nmruntime.NewWorkload(
				ctx, "futures",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					derivatives.NewTrade(ctx),
				}},
			)

			classificationRing := nmruntime.NewWorkload(
				ctx,
				"classification",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					category.NewSolver(ctx),
				}, {
					cognition.NewSolver(ctx),
				}},
			)

			resonanceRing := nmruntime.NewWorkload(
				ctx,
				"resonance",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					resonance.NewSolver(ctx, 1.0),
				}},
			)

			// One training. Nodes in a stage run concurrently against the
			// same envelope, and the grid writes the measurements it is
			// shown — seven of those on one envelope is a concurrent map
			// write. The tape arrives on a channel because walking the
			// record is a long read against an object store.
			tape := measurements(ctx, catalog, runID)

			training := strategy.NewTraining(ctx, tape, instrument, price, balance, api)
			hub.SetTradeStore(training)
			hub.SetExitHandler(training.RequestExit)

			trainerRing := nmruntime.NewWorkload(
				ctx,
				"trainer",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					strategy.NewTraining(ctx, tape),
				}},
			)

			workspace := nmruntime.NewWorkspace(
				ctx,
				"workspace",
				[][]nmruntime.Node[*data.Measurement[float64]]{{
					tickerRing, tradeRing, level3Ring, futuresTicker, futuresTrade,
				}, {
					classificationRing, resonanceRing,
				}, {
					trainerRing,
				}},
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

			if workspace.Status() != nmruntime.READY {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"symm: workspace did not reach ready",
					nil,
				))
			}

			api.MarkReady()

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

/*
configDigest returns a stable digest of the configuration actually loaded for
this run. It hashes the raw bytes of the config file viper resolved; when no
file was used (the embedded default), it returns empty. The digest is what the
Hindsight Run records so replay can distinguish one configuration from another.
*/
func configDigest() string {
	configFile := viper.ConfigFileUsed()

	if configFile == "" {
		return ""
	}

	raw, err := os.ReadFile(configFile)

	if err != nil {
		return ""
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}

/*
buildCodeCommit returns the VCS commit the binary was built from, or the special
"unknown" marker when the build information carries no VCS revision. It never
fabricates a commit string.
*/
func buildCodeCommit() string {
	info, ok := debug.ReadBuildInfo()

	if !ok {
		return "unknown"
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}

	return "unknown"
}

/*
buildBuildID returns a stable build identity from the main module's version and
checksum. When the module carries neither (a from-source `go run`), it falls
back to the Go version that built it, so two different binaries are still
distinguishable without inventing a value.
*/
func buildBuildID() string {
	info, ok := debug.ReadBuildInfo()

	if !ok {
		return "unknown"
	}

	mainVersion := info.Main.Version

	if mainVersion != "" && mainVersion != "(devel)" {
		return mainVersion + "." + info.Main.Sum
	}

	return "go-" + info.GoVersion
}

/*
hindsightSchemaVersions records the wire/Hindsight schema identities needed to
interpret persisted state: the FlatBuffers file identifier (SYMM) and the
Hindsight schema version. These are stable strings, not a fabricated digest.
*/
func hindsightSchemaVersions() map[string]string {
	return map[string]string{
		"wire_file_identifier": "SYMM",
		"hindsight_schema":     "1",
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
		return fmt.Errorf("embedded config file not readable: %w", err)
	}

	defer cfgReader.Close()

	if err := viper.ReadConfig(cfgReader); err != nil {
		return fmt.Errorf("embedded config file not readable: %w", err)
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
