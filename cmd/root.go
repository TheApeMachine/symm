package cmd

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/opportunity"
	"github.com/theapemachine/symm/logic/resonance"
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
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
			// _, err := pyroscope.Start(pyroscope.Config{
			// 	ApplicationName: "symm.theapemachine.app",
			// 	ServerAddress:   "http://localhost:4040",
			// 	Logger:          pyroscope.StandardLogger,
			// })

			// if err != nil {
			// 	log.Fatalf("error starting pyroscope profiler: %v", err)
			// }

			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			errnie.Info(fmt.Sprintf(
				"symm started with %d CPUs", runtime.NumCPU(),
			))

			// startPprof()

			hub := ui.NewHub(cmd.Context())
			defer hub.Close()

			// Phase 1 — the brokers' transport and account objects, which the
			// logic stages and the decision path both consume. The workload
			// maps are built empty here and populated in Phase 2: websocket.New
			// stores the map by reference and only indexes it when envelopes
			// flow, so the maps are complete well before instrument.Subscribe
			// opens the stream.
			publicIngress := map[string]*nmruntime.Workload[*types.Envelope]{}
			privateIngress := map[string]*nmruntime.Workload[*types.Envelope]{}
			futuresIngress := map[string]*nmruntime.Workload[*types.Envelope]{}

			// dataPath resolves ~ and cwd so there is exactly one durable path,
			// in the configured location, not a stray literal-~ directory.
			dataPath := utils.ResolveDataPath()

			// The storage writer is the single CaptureSink for every raw
			// websocket stream (public/private/futures). Each frame is written
			// exactly once, byte-for-byte as it left the wire, tagged with its
			// origin kind (channel/feed) and endpoint. Nothing else is recorded
			// here — raw capture is the irreducible stream, not a re-serialized
			// copy of pipeline state.
			storageEngine, err := store.NewSQLite(filepath.Join(
				dataPath,
				"events.sqlite",
			))

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open storage engine",
					err,
				))
			}

			defer storageEngine.Close()

			// The Hindsight inspection reads (runs / captures / persisted states)
			// are served by the hub over this store.
			hub.SetHindsightStore(storageEngine)

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

			if err := storageEngine.WriteRun(hindsight.RunIdentity{
				StartedAt:      processStartedAt,
				CodeCommit:     buildCodeCommit(),
				BuildID:        buildBuildID(),
				ConfigDigest:   configDigest(),
				SchemaVersions: hindsightSchemaVersions(),
			}.Resolve(runID)); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: persist run identity",
					err,
				))
			}

			// The capture Sequencer mints a stable CaptureIdentity for every
			// raw frame, and the storage writer persists each frame together
			// with that identity — minting and persistence are one step, so raw
			// capture and semantic ingress are joinable by identity, never by
			// timestamp.
			captureSequencer, err := hindsight.NewSequencer(runID)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct capture sequencer",
					err,
				))
			}

			rawCapture, err := store.NewWriter(storageEngine, captureSequencer)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct capture writer",
					err,
				))
			}

			// Witness persistence runs on its own worker: the Disruptor consumer
			// thread enqueues witnesses and never waits on SQLite writes. The
			// bounded queue drops with observability under sustained overflow
			// rather than stalling frame processing.
			asyncWitness := store.NewAsyncWitnessWriter(
				cmd.Context(),
				storageEngine,
				4096,
				50*time.Millisecond,
			)
			defer asyncWitness.Close()

			publicSession := websocket.New(
				cmd.Context(),
				publicIngress,
				websocket.NewSimulator(),
				false,
				system.Cfg.WebSocket.Endpoints.Public,
				rawCapture,
			)

			privateSession := websocket.New(
				cmd.Context(),
				privateIngress,
				websocket.NewSimulator(),
				true,
				system.Cfg.WebSocket.Endpoints.Private,
				rawCapture,
			)

			api := websocket.NewAPI(
				cmd.Context(),
				publicSession,
				privateSession,
			)

			futures := websocket.NewFutures(cmd.Context(), system.Cfg.WebSocket.Endpoints.Futures, futuresIngress, rawCapture)
			api.SetFutures(futures)

			go func() {
				errnie.Error(futures.Run())
			}()

			defer futures.Close()

			recorder, err := audit.NewRecorder(filepath.Join(
				dataPath,
				"audit.jsonl",
			))

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open audit recorder",
					err,
				))
			}

			defer recorder.Close()

			price := broker.NewPrice(api, recorder)
			instrument := broker.NewInstrument(api, price)

			// Reconnect is a soft-reboot of the ONE subscription authority:
			// re-running instrument.Subscribe re-issues the same paced market
			// data batches that boot used. The operational stream epoch is
			// advanced by the transport itself (Live/FuturesLive reconnect
			// handling); Hindsight records that same fact in CaptureIdentity
			// but never supplies it to trading. There is no second subscription
			// path anywhere.
			softReboot := func(endpoint string) func() {
				return func() {
					go func() {
						if err := instrument.Subscribe(); err != nil {
							errnie.Error(errnie.Err(
								errnie.IO,
								"symm: reconnect resubscribe",
								err,
							))
						}
					}()
				}
			}

			publicSession.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Public))
			privateSession.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Private))
			futures.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Futures))

			// Stateful analytical stages are constructed once and mounted directly
			// in each Workload that produces their inputs. The Workloads themselves
			// remain the complete topology; there is no secondary observation store.
			categorySolver := category.NewSolver(cmd.Context())
			cognitionSolver := cognition.NewSolver(cmd.Context())
			opportunitySolver := opportunity.NewSolver(cmd.Context())
			resonanceSolver := resonance.NewSolver(cmd.Context(), 0)

			// The shared manifold solver owns the one resident physics domain.
			// It reads Hawkes excitation fractions on the Trade workload (as
			// forcing state, without advancing the field) and advances on the
			// Level3 workload (applying the latest causally-available forcing).
			manifoldSolver := manifold.NewSolver(cmd.Context())

			// The broker desk owns positions and execution. Recovery adopts the
			// exchange's open inventory before the decision path engages. The
			// position store persists stoploss state across restarts.
			positionStore, err := broker.NewPositionStore(filepath.Join(
				dataPath,
				"positions.sqlite",
			))

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open position store",
					err,
				))
			}

			defer positionStore.Close()

			// The hub serves the trade journal from the position store's
			// persisted position_trades table, so closed trades survive restarts
			// independently of the live ring buffer.
			hub.SetTradeStore(positionStore)

			balance := broker.NewBalance(api)
			positions := &sync.Map{}

			// The shared Perspective store feeds PositionRisk's non-blocking
			// latest-by-key reads. It is constructed before the desk so the
			// guardian path can capture entry snapshots from the same store the
			// advisory layer writes to.
			perspectiveStore := advisor.NewStore()

			recovery := broker.NewRecovery(
				cmd.Context(), api, instrument, price, balance, recorder,
				positionStore, positions, perspectiveStore,
			)

			desk, err := broker.NewDesk(
				cmd.Context(), api, instrument, price, balance, recorder,
				recovery, positionStore, positions, perspectiveStore,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct broker desk",
					err,
				))
			}

			defer desk.Close()

			// Wire the Hindsight trading-lifecycle recorder so real entry/exit
			// transitions persist as first-class, decision-correlated artifacts.
			// It is observational and never affects trading.
			desk.SetLifecycleRecorder(hindsightLifecycleRecorder{engine: storageEngine, runID: runID})

			planner, err := strategy.NewPlanner(cmd.Context(), recorder, desk)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct planner",
					err,
				))
			}

			defer planner.Close()

			// The advisory layer composes the signals' measurements into
			// descriptive Perspectives — context for decision and risk, never
			// decisions. Sixteen families in deterministic order; all instances
			// stay shared across the ticker/trade/Level3/futures workloads.
			advisors := advisorNode{
				store: perspectiveStore,
				advisors: []*advisor.Advisor{
					advisor.NewLiquidityAdvisor("advisor.liquidity"),
					advisor.NewLiquidityDynamicsAdvisor("advisor.liquidity_dynamics"),
					advisor.NewFlowAdvisor("advisor.flow"),
					advisor.NewOrderDispositionAdvisor("advisor.order_disposition"),
					advisor.NewArrivalAdvisor("advisor.arrival"),
					advisor.NewArrivalQualityAdvisor("advisor.arrival_quality"),
					advisor.NewMorphologyAdvisor("advisor.morphology"),
					advisor.NewMorphologyDynamicsAdvisor("advisor.morphology_dynamics"),
					advisor.NewCoordinationAdvisor("advisor.coordination"),
					advisor.NewCoordinationSupportAdvisor("advisor.coordination_support"),
					advisor.NewRelativeStateAdvisor("advisor.relative_state"),
					advisor.NewActivityAdvisor("advisor.activity"),
					advisor.NewDerivativesAdvisor("advisor.derivatives"),
					advisor.NewHistoricalAdvisor("advisor.historical"),
					advisor.NewExecutionAdvisor("advisor.execution"),
					advisor.NewDecompositionAdvisor("advisor.decomposition"),
				},
			}

			// Phase 2 — declare the complete streaming topology as Workloads.
			// The Desk receives priority market/execution updates first. Analytical
			// state then enriches the same envelope before Strategy sees it once.
			publicIngress["ticker"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("ticker.ingress")},
					{&tickNode{desk: desk}},
					{
						system.NewTraced("ticker.correlation", correlation.NewSignal(cmd.Context())),
						system.NewTraced("ticker.leadlag", leadlag.NewSignal(cmd.Context())),
						system.NewTraced("ticker.liquidity", liquidity.NewSignal(cmd.Context())),
						system.NewTraced("ticker.sentiment", sentiment.NewSignal(cmd.Context())),
						system.NewTraced("ticker.pumpdump", pumpdump.NewSignal(cmd.Context())),
						system.NewTraced("ticker.resonance", resonanceSolver),
					},
					{advisors},
					{categorySolver},
					{system.NewTraced("ticker.cognition", cognitionSolver)},
					{system.NewTraced("ticker.opportunity", opportunitySolver)},
					{system.NewDiagnostic("ticker.logic")},
					{planner},
					{system.NewDiagnostic("ticker.strategy")},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("ticker.hub")},
				},
			)

			publicIngress["trade"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("trade.ingress")},
					{
						system.NewTraced("trade.cvd", cvd.NewSignal(cmd.Context(), cvdQuoteProvider(price))),
						system.NewTraced("trade.hawkes", hawkes.NewSignal(cmd.Context())),
						system.NewTraced("trade.pumpdump", pumpdump.NewSignal(cmd.Context())),
						system.NewTraced("trade.toxicity", toxicity.NewSignal(cmd.Context())),
					},
					{system.NewTraced("trade.manifold", manifoldSolver)},
					{advisors},
					{categorySolver},
					{system.NewTraced("trade.cognition", cognitionSolver)},
					{system.NewTraced("trade.opportunity", opportunitySolver)},
					{planner},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("trade.hub")},
				},
			)

			privateIngress["level3"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("level3.ingress")},
					{level3Node{desk: desk}},
					{
						system.NewTraced("level3.depthflow", depthflow.NewSignal(cmd.Context())),
						system.NewTraced("level3.morphology", morphology.NewSignal(cmd.Context())),
						system.NewTraced("level3.pumpdump", pumpdump.NewSignal(cmd.Context())),
						system.NewTraced("level3.toxicity", toxicity.NewSignal(cmd.Context())),
					},
					{system.NewTraced("level3.manifold", manifoldSolver)},
					{advisors},
					{categorySolver},
					{system.NewTraced("level3.cognition", cognitionSolver)},
					{system.NewTraced("level3.opportunity", opportunitySolver)},
					{planner},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("level3.hub")},
				},
			)

			// The private execution stream delivers confirmed fills to the
			// desk. It carries no signal workload: each execution record is
			// routed straight to the matching position's guardian ring, so the
			// workload is a thin dispatch stage plus observability boundaries.
			privateIngress["executions"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("executions.ingress")},
					{executionNode{desk: desk}},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("executions.hub")},
				},
			)

			futuresIngress["ticker"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("futures.ticker.ingress")},
					{system.NewTraced("futures.ticker.derivatives", derivatives.NewSignal(cmd.Context()))},
					{advisors},
					{categorySolver},
					{system.NewTraced("futures.ticker.cognition", cognitionSolver)},
					{system.NewTraced("futures.ticker.opportunity", opportunitySolver)},
					{planner},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("futures.ticker.hub")},
				},
			)

			futuresIngress["trade"] = nmruntime.NewWorkload(
				cmd.Context(),
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("futures.trade.ingress")},
					{system.NewTraced("futures.trade.derivatives", derivatives.NewSignal(cmd.Context()))},
					{advisors},
					{categorySolver},
					{system.NewTraced("futures.trade.cognition", cognitionSolver)},
					{system.NewTraced("futures.trade.opportunity", opportunitySolver)},
					{planner},
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver}},
					{hub},
					{system.NewDiagnostic("futures.trade.hub")},
				},
			)

			workspace := nmruntime.NewWorkspace(
				cmd.Context(),
				append(
					append(
						slices.Collect(maps.Values(publicIngress)),
						slices.Collect(maps.Values(privateIngress))...,
					),
					slices.Collect(maps.Values(futuresIngress))...,
				),
			)

			defer workspace.Close()

			if err := instrument.Subscribe(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: subscribe to instrument universe",
					err,
				))
			}

			// Subscribe returned, so the WHOLE universe is subscribed: the
			// sessions may now feed the trading pipeline. Until this point
			// they were connected and parsing (BUSY) but pushed nothing,
			// because the universe is subscribed in paced batches and the
			// early frames are only whichever symbols were live already.
			api.MarkReady()

			return errors.Join(
				err,
				hub.Run(),
			)
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

	go func() {
		errnie.Error(http.ListenAndServe(addr, nil))
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
