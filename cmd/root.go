package cmd

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	// Registers the /debug/pprof handlers on http.DefaultServeMux, which is
	// the mux startPprof serves. Without it the profiling endpoint answers
	// 404 and the server is dead weight.
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
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
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
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
				Logger:          pyroscope.StandardLogger,
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

			// startPprof()

			hub := ui.NewHub(runtimeCtx)
			defer hub.Close()

			// Phase 1 — the brokers' transport and account objects, which the
			// logic stages and the decision path both consume. The workload
			// maps are built empty here and populated in Phase 2: websocket.New
			// stores the map by reference and only indexes it when envelopes
			// flow, so the maps are complete well before instrument.Subscribe
			// opens the stream.
			publicIngress := map[string]nmruntime.Ingress[*types.Envelope]{}
			privateIngress := map[string]nmruntime.Ingress[*types.Envelope]{}
			futuresIngress := map[string]nmruntime.Ingress[*types.Envelope]{}

			// dataPath resolves ~ and cwd so there is exactly one durable path,
			// in the configured location, not a stray literal-~ directory.
			dataPath := utils.ResolveDataPath()

			// The storage writer is the single CaptureSink for every raw
			// websocket stream (public/private/futures). Each frame is accepted
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
			// raw frame, and the storage writer accepts each frame together with
			// that identity before parsing. Raw capture and semantic ingress are
			// joinable by identity, never by timestamp; the writer persists the
			// ordered queue away from the transport callback.
			captureSequencer, err := hindsight.NewSequencer(runID)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct capture sequencer",
					err,
				))
			}

			rawCapture, err := store.NewWriter(
				storageEngine,
				captureSequencer,
				viper.GetInt("hindsight.capture.queue_depth"),
				viper.GetInt("hindsight.capture.batch_size"),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct capture writer",
					err,
				))
			}

			defer func() {
				errnie.Error(rawCapture.Close())
			}()

			// Witness persistence runs on its own worker: the Disruptor consumer
			// thread enqueues witnesses and never waits on SQLite writes. The
			// bounded queue drops with observability under sustained overflow
			// rather than stalling frame processing.
			asyncWitness := store.NewAsyncWitnessWriter(
				runtimeCtx,
				storageEngine,
				viper.GetInt("hindsight.witness.queue_depth"),
				viper.GetDuration("hindsight.witness.flush_interval"),
			)
			defer asyncWitness.Close()
			witness := newWitnessNode(rawCapture, asyncWitness)

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

			futures := websocket.NewFutures(runtimeCtx, system.Cfg.WebSocket.Endpoints.Futures, futuresIngress, rawCapture)
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

			price := broker.NewPrice(api)
			instrument := broker.NewInstrument(api, price)
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

			momentum := advisor.NewMomentum()
			momentumSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.MomentumName,
				momentum.Features,
			)

			if err := momentumSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct momentum Advisor",
					err,
				))
			}

			defer momentumSolver.Close()

			auction := advisor.NewAuction()
			auctionSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.AuctionName,
				auction.Features,
			)

			if err := auctionSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct auction Advisor",
					err,
				))
			}

			defer auctionSolver.Close()

			participation := advisor.NewParticipation()
			participationSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.ParticipationName,
				participation.Features,
			)

			if err := participationSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct participation Advisor",
					err,
				))
			}

			defer participationSolver.Close()

			pullback := advisor.NewPullback()
			pullbackSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.PullbackName,
				pullback.Features,
			)

			if err := pullbackSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct pullback Advisor",
					err,
				))
			}

			defer pullbackSolver.Close()

			profitRun := advisor.NewProfitRun()
			profitRunSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.ProfitRunName,
				profitRun.Features,
			)

			if err := profitRunSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct profit run Advisor",
					err,
				))
			}

			defer profitRunSolver.Close()

			liquidityAdvisor := advisor.NewLiquidity()
			liquiditySolver := advisor.NewSolver(
				runtimeCtx,
				advisor.LiquidityName,
				liquidityAdvisor.Features,
			)

			if err := liquiditySolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct liquidity Advisor",
					err,
				))
			}

			defer liquiditySolver.Close()

			basisAdvisor := advisor.NewBasis()
			basisSolver := advisor.NewSolver(
				runtimeCtx,
				advisor.BasisName,
				basisAdvisor.Features,
			)

			if err := basisSolver.Error(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"symm: construct basis Advisor",
					err,
				))
			}

			defer basisSolver.Close()

			// Manifold is intentionally disabled while its runtime issue is
			// investigated independently. No physics implementation is changed.
			// manifoldSolver := manifold.NewSolver(runtimeCtx)
			// hub.SetManifoldSnapshot(manifoldSolver.Snapshot)
			pumpdumpSolver := pumpdump.NewSignal(runtimeCtx, cvdQuoteProvider(price))
			toxicitySolver := toxicity.NewSignal(runtimeCtx)
			derivativesSolver := derivatives.NewSignal(runtimeCtx)

			// The broker desk owns positions and execution. Recovery adopts the
			// exchange's open inventory before the decision path engages. The
			// position store persists stoploss state across restarts.
			positionStore, err := broker.NewPositionStore(filepath.Join(
				dataPath,
				"positions.sqlite",
			),
				viper.GetInt("trading.persistence.queue_depth"),
				viper.GetInt("trading.persistence.batch_size"),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: open position store",
					err,
				))
			}

			defer func() {
				errnie.Error(positionStore.Close())
			}()

			// The hub serves the trade journal from the position store's
			// persisted position_trades table, so closed trades survive restarts
			// independently of the live ring buffer.
			hub.SetTradeStore(positionStore)

			balance := broker.NewBalance(api)
			positions := &sync.Map{}

			recovery := broker.NewRecovery(
				runtimeCtx, api, instrument, price, balance,
				positionStore, positions,
			)

			desk, err := broker.NewDesk(
				runtimeCtx, api, instrument, price, balance,
				recovery, positionStore, positions,
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
			// It observes transitions after they occur and persists away from the
			// guardian through the shared storage writer.
			desk.SetLifecycleRecorder(hindsightLifecycleRecorder{writer: rawCapture, runID: runID})

			planner, err := strategy.NewPlanner(runtimeCtx, desk)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct planner",
					err,
				))
			}

			defer planner.Close()

			basisArena, err := advisor.NewArena(
				advisor.BasisName,
				basisAdvisor.Features,
				planner,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct basis Arena",
					err,
				))
			}

			liquidityArena, err := advisor.NewArena(
				advisor.LiquidityName,
				liquidityAdvisor.Features,
				basisArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct liquidity Arena",
					err,
				))
			}

			pullbackArena, err := advisor.NewArena(
				advisor.PullbackName,
				pullback.Features,
				liquidityArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct pullback Arena",
					err,
				))
			}

			profitRunArena, err := advisor.NewArena(
				advisor.ProfitRunName,
				profitRun.Features,
				pullbackArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct profit run Arena",
					err,
				))
			}

			participationArena, err := advisor.NewArena(
				advisor.ParticipationName,
				participation.Features,
				profitRunArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct participation Arena",
					err,
				))
			}

			auctionArena, err := advisor.NewArena(
				advisor.AuctionName,
				auction.Features,
				participationArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct auction Arena",
					err,
				))
			}

			momentumArena, err := advisor.NewArena(
				advisor.MomentumName,
				momentum.Features,
				auctionArena,
				len(instrument.Symbols()),
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct momentum Arena",
					err,
				))
			}

			// Every Arena reports its resolved predictions into the Planner's
			// credibility ledger. This is the Court of Causal Accountability
			// (MCTS.md §6): the Arena is the only stage that observes whether a
			// falsifiable claim was borne out, so without this link the ledger
			// was written once at construction and never again — every advisor
			// held credibility 1.0 forever and being wrong cost nothing.
			court := planner.Court()

			// No Arena decides anything until the system is fully booted.
			// instrument.Subscribe walks the symbol universe in paced batches,
			// so the first batch streams live while later batches are still
			// connecting. Classifiers fed that partial universe are cold, their
			// distributions are uniform, and a uniform distribution has no
			// winner to act on. Gating on instrument READY means the advisors
			// stay silent until the whole universe is subscribed.
			booted := func() bool {
				return instrument.Status() == nmruntime.READY
			}

			for _, arena := range []*advisor.Arena{
				momentumArena, auctionArena, participationArena, profitRunArena,
				pullbackArena, liquidityArena, basisArena,
			} {
				arena.Court(court).Booted(booted)
			}

			// Phase 2 — declare the complete streaming topology as Workloads.
			// The Desk receives priority market/execution updates first. Analytical
			// state then enriches the same envelope before Strategy sees it once.
			publicTicker := nmruntime.NewWorkload(
				runtimeCtx,
				"ticker",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("ticker.ingress")},
					{&tickNode{desk: desk}},
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
						system.NewTraced("trade.cvd", cvd.NewSignal(runtimeCtx, cvdQuoteProvider(price))),
						system.NewTraced("trade.hawkes", hawkes.NewSignal(runtimeCtx)),
					},
				},
			)

			privateLevel3 := nmruntime.NewWorkload(
				runtimeCtx,
				"level3",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("level3.ingress")},
					{level3Node{desk: desk}},
					{
						system.NewTraced("level3.depthflow", depthflow.NewSignal(runtimeCtx)),
						system.NewTraced("level3.morphology", morphology.NewSignal(runtimeCtx)),
					},
				},
			)

			// The private execution stream delivers confirmed fills to the
			// desk. It carries no signal workload: each execution record is
			// routed straight to the matching position's guardian ring, so the
			// workload is a thin dispatch stage plus observability boundaries.
			privateExecutions := nmruntime.NewWorkload(
				runtimeCtx,
				"executions",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewDiagnostic("executions.ingress")},
					{executionNode{desk: desk}},
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

			pumpdumpWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"pumpdump",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewTraced("pumpdump.signal", pumpdumpSolver)},
				},
			)

			toxicityWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"toxicity",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewTraced("toxicity.signal", toxicitySolver)},
				},
			)

			derivativesWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"derivatives",
				[][]nmruntime.Node[*types.Envelope]{
					{system.NewTraced("derivatives.signal", derivativesSolver)},
				},
			)

			// manifoldWorkload := nmruntime.NewWorkload(
			// 	runtimeCtx,
			// 	"manifold",
			// 	[][]nmruntime.Node[*types.Envelope]{
			// 		{system.NewTraced("manifold.advance", manifoldSolver)},
			// 	},
			// )

			advisorWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"advisor",
				[][]nmruntime.Node[*types.Envelope]{
					{
						system.NewTraced("advisor.momentum", momentumSolver),
						system.NewTraced("advisor.auction", auctionSolver),
						system.NewTraced("advisor.participation", participationSolver),
						system.NewTraced("advisor.pullback", pullbackSolver),
						system.NewTraced("advisor.profit_run", profitRunSolver),
						system.NewTraced("advisor.liquidity", liquiditySolver),
						system.NewTraced("advisor.basis", basisSolver),
					},
				},
			)

			logicWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"logic",
				[][]nmruntime.Node[*types.Envelope]{
					{
						system.NewTraced("logic.category", categorySolver),
					},
					{
						system.NewTraced("logic.cognition", cognitionSolver),
					},
					{system.NewDiagnostic("logic.complete")},
				},
			)

			strategyWorkload := nmruntime.NewWorkload(
				runtimeCtx,
				"strategy",
				[][]nmruntime.Node[*types.Envelope]{
					{momentumArena},
					{&deskContextNode{desk: desk}},
					{system.NewDiagnostic("strategy.planned")},
					{witness},
					{hub},
					{system.NewDiagnostic("strategy.published")},
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
					{pumpdumpWorkload, toxicityWorkload, derivativesWorkload},
					{advisorWorkload, logicWorkload},
					{strategyWorkload},
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

			// Every consumer must be able to accept an envelope before any market
			// subscription is written. The connected transports remain BUSY and
			// deliberately discard market frames until both runtime layers have
			// crossed their READY boundary.
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

			// Subscription snapshots are authoritative initial state, especially
			// for Level3 order lifecycles. Subscribe only after READY so the first
			// snapshot and every following delta traverse the same live pipeline.
			if err := instrument.Subscribe(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: subscribe to instrument universe",
					err,
				))
			}

			hubErrors := make(chan error, 1)

			go func() {
				hubErrors <- hub.Run()
			}()

			select {
			case <-rawCapture.Failed():
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: capture storage failed",
					rawCapture.Error(),
				))
			case <-positionStore.Failed():
				return errnie.Error(errnie.Err(
					errnie.IO,
					"symm: position storage failed",
					positionStore.Error(),
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
