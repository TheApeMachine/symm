package cmd

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/theapemachine/symm/strategy"
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
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
	"github.com/theapemachine/symm/store"
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
		Short: "S.Y.M.M. is not financial advice.",
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

			runtimeCtx, runtimeCancel := context.WithCancel(cmd.Context())
			defer runtimeCancel()

			startPprof()

			hub := ui.NewHub(runtimeCtx)
			defer hub.Close()

			// The hub reads the ring rather than running on it: a publisher
			// mounted as a stage would encode every frame on the ring's own
			// goroutine at ingress rate. The sink's only obligation is one
			// channel send, and the hub owns its work on a goroutine of its
			// own. A full buffer drops, which for a live view is the right
			// answer — it shows the present, not a backlog.
			uiSink := nmruntime.NewSink[*types.Envelope](128)
			hub.Consume(uiSink.Out())

			// Phase 1 — the brokers' transport and account objects, which the
			// logic stages and the decision path both consume. The workload
			// maps are built empty here and populated in Phase 2: websocket.New
			// stores the map by reference and only indexes it when envelopes
			// flow, so the maps are complete well before instrument.Subscribe
			// opens the stream.
			publicIngress := map[string]nmruntime.Ingress[*types.Envelope]{}
			privateIngress := map[string]nmruntime.Ingress[*types.Envelope]{}
			futuresIngress := map[string]nmruntime.Ingress[*types.Envelope]{}

			// The storage writer is the single CaptureSink for every raw
			// websocket stream (public/private/futures). Each frame is accepted
			// exactly once, byte-for-byte as it left the wire, tagged with its
			// origin kind (channel/feed) and endpoint. Nothing else is recorded
			// here — raw capture is the irreducible stream, not a re-serialized
			// copy of pipeline state.
			storageStarted := time.Now()
			errnie.Info("store: opening S3 archive")
			storageEngine, err := store.NewS3(cmd.Context())

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open storage engine",
					err,
				))
			}

			errnie.Info(fmt.Sprintf("store: S3 archive ready after %s", time.Since(storageStarted)))
			defer func() {
				if err := storageEngine.Close(); err != nil {
					errnie.Error(err)
				}
			}()
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

			rawCapture, err := recording.NewSession(runtimeCtx, writer, hindsight.RunIdentity{
				StartedAt: processStartedAt, CodeCommit: buildCodeCommit(), BuildID: buildBuildID(), ConfigDigest: configDigest(), SchemaVersions: hindsightSchemaVersions(),
			}.Resolve(runID), viper.GetInt("hindsight.capture.batch_size"), viper.GetDuration("hindsight.capture.flush_interval"))
			if err != nil {
				return err
			}
			defer func() {
				if err := rawCapture.Close(); err != nil {
					errnie.Error(err)
				}
			}()

			publicSession := websocket.New(
				runtimeCtx,
				publicIngress,
				websocket.NewSimulator(),
				false,
				system.Cfg.WebSocket.Endpoints.Public,
				rawCapture,
			)

			defer publicSession.Close()

			if err := publicSession.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: public transport failed during construction",
					err,
				))
			}

			privateSession := websocket.New(
				runtimeCtx,
				privateIngress,
				websocket.NewSimulator(),
				true,
				system.Cfg.WebSocket.Endpoints.Private,
				rawCapture,
			)
			defer privateSession.Close()

			if err := privateSession.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: private transport failed during construction",
					err,
				))
			}

			api := websocket.NewAPI(
				runtimeCtx,
				publicSession,
				privateSession,
			)

			defer api.Close()

			futures := websocket.NewFutures(
				runtimeCtx, system.Cfg.WebSocket.Endpoints.Futures, futuresIngress, rawCapture,
			)

			api.SetFutures(futures)

			transportErrors := make(chan error, 1)

			go func() {
				transportErrors <- api.Run()
				runtimeCancel()
			}()

			if err := api.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: transport failed during construction",
					err,
				))
			}

			instrument := broker.NewInstrument(api)
			price := broker.NewPrice(api, instrument)
			balance := broker.NewBalance(api)
			go balance.Run(runtimeCtx, instrument)

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
			categorySolver := category.NewSolver(runtimeCtx)
			cognitionSolver := cognition.NewSolver(runtimeCtx)
			resonanceSolver := resonance.NewSolver(runtimeCtx, 0)
			resonanceSolver.SetObserver(hub.PublishResonance)

			manifoldSolver := manifold.NewSolver(runtimeCtx)
			// The Level3 stream carries a semaphore, not orders: the venue's
			// book is the population, and the advance reads it directly.
			manifoldSolver.SetBooks(api)
			defer manifoldSolver.Close()

			manifoldSolver.SetViewer(hub)
			manifoldSolver.Start()

			pumpdumpSolver := pumpdump.NewSignal(runtimeCtx, api)
			toxicitySolver := toxicity.NewSignal(runtimeCtx)
			derivativesSolver := derivatives.NewSignal(runtimeCtx)

			// These producers run synchronously at their transport owner rather
			// than as workload stages, so they are traced explicitly: an
			// untraced node stamps no boundary and is invisible in the
			// diagnostics topology even while it is producing measurements.
			privateSession.Level3Observers = func() []nmruntime.Node[*types.Envelope] {
				return []nmruntime.Node[*types.Envelope]{
					system.NewTraced("level3.depthflow", depthflow.NewSignal(runtimeCtx)),
					system.NewTraced("level3.morphology", morphology.NewSignal(runtimeCtx)),
					system.NewTraced("level3.pumpdump", pumpdumpSolver),
					system.NewTraced("level3.toxicity", toxicitySolver),
				}
			}

			if err := price.GetFees(instrument.Symbols()); err != nil {
				return err
			}
			if balance.Status() != types.READY {
				return errnie.Error(errnie.Err(errnie.NotAcceptable, "symm: initial balance is not ready", nil))
			}

			learner, err := strategy.NewLearner(runtimeCtx, api, price, balance, system.Cfg.Learning.Traders, catalog, storageEngine, runID, rawCapture)

			if err != nil {
				return err
			}
			defer func() {
				if err := learner.Population.Save(context.Background(), learner.Checkpoint); err != nil {
					errnie.Error(err)
				}
			}()
			go learner.Run(runtimeCtx, system.Cfg.Learning.CheckpointInterval)

			// The workspace owns the complete forward-learning loop. Signal and
			// logic producers finish before the shared grid and action owner run.
			publicTicker := nmruntime.NewWorkload(
				runtimeCtx,
				"ticker",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("ticker.ingress")},

					{
						system.NewTraced("ticker.correlation", correlation.NewSignal(runtimeCtx)),
						system.NewTraced("ticker.leadlag", leadlag.NewSignal(runtimeCtx)),
						system.NewTraced("ticker.liquidity", liquidity.NewSignal(runtimeCtx)),
						system.NewTraced("ticker.sentiment", sentiment.NewSignal(runtimeCtx)),
						system.NewTraced("ticker.resonance", resonanceSolver),
					},
				},
			)

			publicTrade := nmruntime.NewWorkload(
				runtimeCtx,
				"trade",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("trade.ingress")},
					{
						system.NewTraced("trade.cvd", cvd.NewSignal(runtimeCtx, func(symbol string) (*decimal.Decimal, *decimal.Decimal) {
							tick := price.Tick(symbol)
							if tick == nil {
								return nil, nil
							}
							return tick.Bid, tick.Ask
						})),
						system.NewTraced("trade.hawkes", hawkes.NewSignal(runtimeCtx)),
					},
					{
						system.NewTraced("trade.manifold", manifoldSolver),
					},
				},
			)

			privateLevel3 := nmruntime.NewWorkload(
				runtimeCtx,
				"level3",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("level3.ingress")},

					{
						system.NewTraced("level3.manifold", manifoldSolver),
					},
				},
			)

			// Account execution notifications remain observable. The learning
			// wallets execute independently against the shared displayed book.
			privateExecutions := nmruntime.NewWorkload(
				runtimeCtx,
				"executions",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("executions.ingress")},
				},
			)

			futuresTicker := nmruntime.NewWorkload(
				runtimeCtx,
				"futures.ticker",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("futures.ticker.ingress")},
				},
			)

			futuresTrade := nmruntime.NewWorkload(
				runtimeCtx,
				"futures.trade",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("futures.trade.ingress")},
				},
			)

			workspace := nmruntime.NewWorkspace(
				runtimeCtx,
				"workspace",
				[][]nmruntime.Node[*types.Envelope]{
					{
						publicTicker,
						publicTrade,
						privateLevel3,
						privateExecutions,
						futuresTicker,
						futuresTrade,
					},
					// These dependent numerical steps share one event turn. Separate
					// polling barriers otherwise spend more time scheduling these
					// short steps than processing them under sustained backpressure.
					{system.NewTraced("logic.pumpdump", pumpdumpSolver), system.NewTraced("logic.toxicity", toxicitySolver), system.NewTraced("logic.derivatives", derivativesSolver)},
					{system.NewTraced("logic.category", categorySolver)},
					{system.NewTraced("logic.cognition", cognitionSolver)},
					{system.NewTraced("learning", learner)},
					{uiSink},
				},
			)

			defer workspace.Close()

			if err := workspace.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct workspace",
					err,
				))
			}

			publicIngress["ticker"] = publicTicker
			publicIngress["trade"] = publicTrade
			privateIngress["level3"] = privateLevel3
			privateIngress["executions"] = privateExecutions
			futuresIngress["ticker"] = futuresTicker
			futuresIngress["trade"] = futuresTrade

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
				return errnie.Error(errnie.Err(errnie.NotAcceptable, "symm: instrument universe is not seeded", nil))
			}

			workspace.Admit()

			if workspace.Status() == nil ||
				workspace.Status().Current() != nmruntime.READY {
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

			hubErrors := make(chan error, 1)

			go func() {
				hubErrors <- hub.Run()
			}()

			select {
			case err := <-rawCapture.Errors:
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: capture storage failed",
					err,
				))
			case err := <-transportErrors:

				if api.Error() == nil && cmd.Context().Err() != nil {
					return cmd.Context().Err()
				}

				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: required transport failed",
					err,
				))
			case err := <-hubErrors:
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: dashboard server failed",
					err,
				))
			case <-runtimeCtx.Done():

				if err := api.Error(); err != nil {
					return err
				}

				if err := cmd.Context().Err(); err != nil {
					return err
				}

				return runtimeCtx.Err()
			}
		},
	}
)

/*
Register attaches an external subcommand to the root command.
*/
func Register(command *cobra.Command) {
	rootCmd.AddCommand(command)
}

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
