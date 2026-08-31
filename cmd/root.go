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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/opportunity"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/data"
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

/*
tickNode drives the live decision clock off the ticker stream. One market
ticker observation commits an engine tick, advances the broker desk's live
marks, and runs the strategy planner's portfolio pass. It is the single place
the thesis clock advances, so the resonance, graph, and causal stages all read
one coherent tick watermark.
*/
type tickNode struct {
	thesis  *types.Thesis
	desk    *broker.Desk
	planner *strategy.Planner
}

func (node tickNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	envelope.Tick = node.thesis.AdvanceTick(envelope.TickerData.Timestamp)

	if err := node.desk.StepTicker(envelope.TickerData); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"symm: desk ticker step",
			err,
		))
	}

	envelope.StrategyRound = node.planner.StepTick(envelope.TickerData)

	// Stamp the account valuation the desk has most recently published, so the
	// dashboard's balance rides the market stream it is already consuming. The
	// snapshot is whatever the broker last reported: this reads it, never polls
	// for it, so a tick costs nothing extra when the valuation has not changed.
	if balance, _, found := node.thesis.EquitySnapshot(); found {
		envelope.Equity = types.NewEquityReading(balance)
	}

	// Stamp the open-lot set so the positions panel recovers from the same
	// market event as equity, rather than a broker poll the dashboard has no
	// way to trigger.
	envelope.Positions = node.desk.OpenPositionWire()

	return envelope
}

func (node tickNode) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

/*
level3Node routes one parsed Level3 frame to the broker desk so the canonical
per-symbol execution state is advanced and the matching open position's
guardian can evaluate the post-frame executable surface. It is the Level3
analogue of executionNode: each frame lands on this node and is applied to the
desk's continuously-resident execution reducer before any signal or downstream
stage reads it.
*/
type level3Node struct {
	desk *broker.Desk
}

func (node level3Node) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeLevel3 || node.desk == nil {
		return envelope
	}

	if err := node.desk.StepLevel3Epoch(
		envelope.Level3Data,
		uint64(envelope.Stream.Epoch),
	); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"symm: desk level3 step",
			err,
		))
	}

	return envelope
}

func (node level3Node) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

/*
executionNode routes one confirmed execution record to the broker desk so the
matching open position can advance its fill state. It is the account-execution
analogue of tickNode: the private execution stream's frames land on this node
and are dispatched to the desk's serial guardian ring, never shared with the
public market-data pipeline.
*/
type executionNode struct {
	desk *broker.Desk
}

func (node executionNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeExecution || node.desk == nil {
		return envelope
	}

	if err := node.desk.StepExecution(envelope.ExecutionData); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"symm: desk execution step",
			err,
		))
	}

	return envelope
}

/*
strategyNode is the strategy layer's stream boundary. It receives the envelope
the logic layer finished — reading the GraphUpdate the graph stage just stamped
— and hands that symbol/at to the reasoner so it re-fits its causal transition
models from the shared store the graph stage already advanced. Because it sits
after the logic stages and before tickNode, the reasoner observes a fully-formed
logic result once per envelope, in stream order, and never reaches sideways.
*/
type strategyNode struct {
	planner *strategy.Planner
}

func (node strategyNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.GraphUpdate == nil || node.planner == nil {
		return envelope
	}

	node.planner.Reasoner().OnGraphUpdate(
		envelope.GraphUpdate.Symbol,
		envelope.GraphUpdate.At,
	)

	return envelope
}

func (node strategyNode) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

/*
advisorNode is the advisory layer's stream boundary. It composes this envelope's
signal measurements through the declared advisor families and attaches the
resulting Perspectives — descriptive context, never decisions. Each advisor sees
the measurements it declares once, in stream order, and produces a bounded
resident reading; the perspectives then flow to whatever consumes them
(decision context and position/risk management), not to the UI as a gate.
*/
type advisorNode struct {
	advisors []*advisor.Advisor
	store    *advisor.Store
}

func (node advisorNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil {
		return envelope
	}

	var perspectives []*types.Perspective

	for _, advisorInstance := range node.advisors {
		for _, measurement := range envelope.SignalMeasurements() {
			if measurement == nil {
				continue
			}

			if perspective := advisorInstance.Step(measurement); perspective != nil {
				perspectives = append(perspectives, perspective)

				if node.store != nil {
					node.store.Put(perspective)
				}
			}
		}
	}

	if len(perspectives) > 0 {
		envelope.Perspectives = perspectives
	}

	return envelope
}

func (node advisorNode) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

/*
cvdQuoteProvider yields the shared top-of-book quote for the CVD signal's
response-price metrics, read synchronously from the broker desk's live tick
cache. It restores the contemporaneous-quote dependency that was severed during
the workspace refactor, so midpoint_log_return (the causal outcome) is once
again computable from real market data rather than permanently undefined.
*/
func cvdQuoteProvider(price *broker.Price) func(symbol string) (bid, ask *decimal.Decimal) {
	return func(symbol string) (bid, ask *decimal.Decimal) {
		if price == nil {
			return nil, nil
		}

		tick := price.Tick(symbol)

		if tick == nil {
			return nil, nil
		}

		return tick.Bid, tick.Ask
	}
}

/*
witnessNode is the Hindsight witness boundary of the live pipeline. It observes
the semantic artifacts the running binary actually produced on an envelope —
Categories and signal Measurements — and records an ArtifactWitness for each,
keyed to the exact EnvelopeRef (CaptureIdentity + ordinal) that carried it. The
witness is historical evidence: what actually ran, not what replay would produce.
*/
type witnessNode struct {
	writer         *store.Writer
	asyncWriter    *store.AsyncWitnessWriter
	categorySolver *category.Solver
	graphSolver    *graph.Solver
}

func (node witnessNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || (node.writer == nil && node.asyncWriter == nil) {
		return envelope
	}

	if !envelope.CaptureID.Valid() {
		return envelope
	}

	ref := hindsight.EnvelopeRef{
		Origin:  envelope.CaptureID,
		Ordinal: envelope.CaptureOrdinal,
	}

	// One "state" witness per envelope persists the complete EnvelopeState — the
	// exact system state at this Observe boundary — so inspection can scrub to
	// any capture and see precisely what the running binary produced, joined back
	// to its raw frame by the capture provenance carried in the state itself.
	//
	// The payload encoding stays on the producing thread (it reads the in-flight
	// envelope, which is not safe to touch after Step returns); the persistence
	// itself is offloaded. With an async writer the bytes are handed to a
	// background worker and never block the Disruptor consumer on SQLite writes.
	stateWitness := hindsight.ArtifactWitness{
		Envelope: ref,
		Boundary: "observe",
		Artifact: hindsight.ArtifactID{
			Kind:     "state",
			Identity: string(ref.Origin.Run) + ":" + strconv.FormatUint(uint64(ref.Origin.Sequence), 10) + ":" + strconv.FormatUint(ref.Ordinal, 10),
		},
		ImmediateParents: nil,
		Payload:          envelope.EncodeBytes(),
	}

	node.write(stateWitness)

	for _, category := range envelope.Categories {
		node.record(
			ref,
			"after-category",
			"category",
			string(category.Type),
			[]hindsight.EnvelopeRef{ref},
			"category",
			node.categoryVersion(),
		)
	}

	measurements := []*data.Measurement[float64]{
		envelope.CVD,
		envelope.Hawkes,
		envelope.DepthFlow,
		envelope.Morphology,
		envelope.Liquidity,
		envelope.PumpDump,
		envelope.Toxicity,
		envelope.Derivatives,
		envelope.Correlation,
		envelope.LeadLag,
		envelope.Sentiment,
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		node.record(
			ref,
			"after-signals",
			"measurement",
			measurement.ID,
			[]hindsight.EnvelopeRef{ref},
			"graph",
			node.graphVersion(),
		)
	}

	// The strategy decision round is a first-class historical artifact: what
	// the planner actually decided on this envelope, keyed to the same exact
	// EnvelopeRef so an inspector can trace a decision back to the semantic
	// state that produced it. This is observational — recording never alters
	// trading, and a nil/empty round is simply not recorded.
	if envelope.StrategyRound != nil {
		for _, decision := range envelope.StrategyRound.Decisions {
			if decision == nil {
				continue
			}

			node.recordDecision(ref, decision)
		}
	}

	return envelope
}

/*
recordDecision persists one planner decision as a first-class historical
artifact. Its identity is the decision's own ID (stable across the trade), and
its semantic parents are the decision's declared perspective sources plus its
causal identification — the exact shared inputs that produced it, named so a
consumer can traverse each source to its own witness by identity.
*/
func (node witnessNode) recordDecision(ref hindsight.EnvelopeRef, decision *types.Decision) {
	if decision == nil || decision.ID == "" {
		return
	}

	semanticParents := make([]string, 0, len(decision.PerspectiveSources)+1)

	for _, source := range decision.PerspectiveSources {
		semanticParents = append(semanticParents, source.Source)
	}

	if decision.CausalIdentification != "" {
		semanticParents = append(semanticParents, decision.CausalIdentification)
	}

	witness := hindsight.ArtifactWitness{
		Envelope: ref,
		Boundary: "after-planner",
		Artifact: hindsight.ArtifactID{
			Kind:     "decision",
			Identity: decision.ID,
		},
		Component:             "planner",
		ComponentStateVersion: node.graphVersion(),
		ImmediateParents:      []hindsight.EnvelopeRef{ref},
		SemanticParents:       semanticParents,
	}

	node.write(witness)
}

func (node witnessNode) categoryVersion() uint64 {
	if node.categorySolver == nil {
		return 0
	}

	return node.categorySolver.Version()
}

func (node witnessNode) graphVersion() uint64 {
	if node.graphSolver == nil || node.graphSolver.Store() == nil {
		return 0
	}

	return node.graphSolver.Store().Version()
}

func (node witnessNode) record(
	ref hindsight.EnvelopeRef,
	boundary, kind, identity string,
	parents []hindsight.EnvelopeRef,
	component string,
	stateVersion uint64,
) {
	if identity == "" {
		return
	}

	node.write(hindsight.ArtifactWitness{
		Envelope: ref,
		Boundary: boundary,
		Artifact: hindsight.ArtifactID{
			Kind:     kind,
			Identity: identity,
		},
		Component:             component,
		ComponentStateVersion: stateVersion,
		ImmediateParents:      parents,
		Payload:               nil,
	})
}

/*
write routes one artifact witness to either the asynchronous background worker
(live hot path) or the synchronous transport writer (tests and any caller that
did not wire an async writer). It never blocks the producing thread when the
async path is active: backpressure is the async writer's bounded queue, whose
overflow policy drops with observability rather than stalling the pipeline.
*/
func (node witnessNode) write(witness hindsight.ArtifactWitness) {
	if node.asyncWriter != nil {
		node.asyncWriter.Enqueue(witness)
		return
	}

	_ = node.writer.WriteWitness(witness)
}

func (node witnessNode) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

/*
hindsightLifecycleRecorder adapts the broker's LifecycleRecorder seam to the
Hindsight store. It turns one trading-lifecycle transition into a durable
LifecycleEvent keyed by the decision ID, tagged with the run. Recording is
fire-and-forget: a persistence failure is logged and never blocks the trade.
*/
type hindsightLifecycleRecorder struct {
	engine *store.SQLite
	runID  hindsight.RunID
}

func (recorder hindsightLifecycleRecorder) RecordLifecycle(event hindsight.LifecycleEvent) {
	if recorder.engine == nil || event.DecisionID == "" || event.Kind == "" {
		return
	}

	if err := recorder.engine.WriteLifecycleEvent(recorder.runID, event); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"symm: record lifecycle event",
			err,
		))
	}
}

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

			thesis := types.NewThesis(cmd.Context())

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
					if err := instrument.Subscribe(); err != nil {
						errnie.Error(errnie.Err(
							errnie.IO,
							"symm: reconnect resubscribe",
							err,
						))
					}
				}
			}

			publicSession.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Public))
			privateSession.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Private))
			futures.SetReconnect(softReboot(system.Cfg.WebSocket.Endpoints.Futures))

			// The shared graph stage owns the authoritative observation store
			// and influence graph. Both the envelope pipeline (which folds
			// measurements into it) and the strategy reasoner (which reads it)
			// bind this single instance — there is no second estimator.
			epoch := uint64(1)
			measurementStep := system.Cfg.Snapshot().Planner.MeasurementStep
			schemaTemplate := strategy.DefaultCausalSchema(epoch, measurementStep)
			graphSolver := graph.NewSolver(
				cmd.Context(),
				epoch,
				2048,
				strategy.RelationPlansFromSchema(schemaTemplate, epoch, system.Cfg.Snapshot().Planner.RelationMaxLag),
				schemaTemplate.Version,
				graph.WithInterval(system.Cfg.Snapshot().Planner.RelationInterval),
			)

			// The shared category solver owns the authoritative per-symbol
			// evidence snapshot. It and the graph solver and the advisory layer
			// are mounted at every producer Workload that delivers one of their
			// declared inputs, so trade (CVD/Hawkes) and Level3
			// (DepthFlow/Morphology) measurements reach the same semantic state
			// instead of being stranded on their own rings.
			categorySolver := category.NewSolver(cmd.Context())

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
				cmd.Context(), api, instrument, price, balance, thesis, recorder,
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

			// The strategy planner is the one authoritative live decision path:
			// it consumes the graph's fitted influence state, runs economic
			// MCTS per symbol, and executes through the desk.
			planner := strategy.NewPlanner(
				cmd.Context(), thesis, recorder, desk,
				graphSolver.Store(), graphSolver.Graph(),
			)

			if planner == nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: construct strategy planner",
					nil,
				))
			}

			defer planner.Close()

			// The advisory layer composes the signals' measurements into
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

			// Phase 2 — populate the workload maps now that every shared
			// dependency exists. The ticker stage folds measurements into the
			// shared graph, resolves categories, cognition, opportunity, and
			// causal readings, and advances the desk/planner on the thesis tick.
			publicIngress["ticker"] = nmruntime.NewWorkload(
				cmd.Context(),
				append(
					[][]nmruntime.Node[*types.Envelope]{
						{system.NewDiagnostic("ticker.ingress")},
						{
							system.NewTraced("ticker.correlation", correlation.NewSignal(cmd.Context())),
							system.NewTraced("ticker.leadlag", leadlag.NewSignal(cmd.Context())),
							system.NewTraced("ticker.liquidity", liquidity.NewSignal(cmd.Context())),
							system.NewTraced("ticker.sentiment", sentiment.NewSignal(cmd.Context())),
							system.NewTraced("ticker.pumpdump", pumpdump.NewSignal(cmd.Context())),
							system.NewTraced("ticker.resonance", resonance.NewSolver(cmd.Context(), 0, thesis)),
						},
					},
					append(
						semanticCore("ticker", advisors, graphSolver, categorySolver),
						[][]nmruntime.Node[*types.Envelope]{
							{system.NewDiagnostic("ticker.category")},
							{
								system.NewTraced("ticker.cognition", cognition.NewSolver(cmd.Context(), thesis)),
								system.NewTraced("ticker.opportunity", opportunity.NewSolver(cmd.Context())),
							},
							{system.NewDiagnostic("ticker.logic")},
							{strategyNode{planner: planner}},
							{tickNode{thesis: thesis, desk: desk, planner: planner}},
							{system.NewDiagnostic("ticker.trade")},
							{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
							{hub},
							{system.NewDiagnostic("ticker.hub")},
						}...,
					)...,
				),
			)

			publicIngress["trade"] = nmruntime.NewWorkload(
				cmd.Context(),
				append(
					[][]nmruntime.Node[*types.Envelope]{
						{system.NewDiagnostic("trade.ingress")},
						{
							system.NewTraced("trade.cvd", cvd.NewSignal(cmd.Context(), cvdQuoteProvider(price))),
							system.NewTraced("trade.hawkes", hawkes.NewSignal(cmd.Context())),
							system.NewTraced("trade.pumpdump", pumpdump.NewSignal(cmd.Context())),
							system.NewTraced("trade.toxicity", toxicity.NewSignal(cmd.Context())),
						},
						{system.NewTraced("trade.manifold", manifoldSolver)},
					},
					append(
						semanticCoreLight("trade", advisors, categorySolver),
						[][]nmruntime.Node[*types.Envelope]{
							{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
							{hub},
							{system.NewDiagnostic("trade.hub")},
						}...,
					)...,
				),
			)

			privateIngress["level3"] = nmruntime.NewWorkload(
				cmd.Context(),
				append(
					[][]nmruntime.Node[*types.Envelope]{
						{system.NewDiagnostic("level3.ingress")},
						{level3Node{desk: desk}},
						{
							system.NewTraced("level3.depthflow", depthflow.NewSignal(cmd.Context())),
							system.NewTraced("level3.morphology", morphology.NewSignal(cmd.Context())),
							system.NewTraced("level3.pumpdump", pumpdump.NewSignal(cmd.Context())),
							system.NewTraced("level3.toxicity", toxicity.NewSignal(cmd.Context())),
						},
						{
							system.NewTraced("level3.manifold", manifoldSolver),
						},
					},
					append(
						semanticCoreLight("level3", advisors, categorySolver),
						[][]nmruntime.Node[*types.Envelope]{
							{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
							{hub},
							{system.NewDiagnostic("level3.hub")},
						}...,
					)...,
				),
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
					{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
					{hub},
					{system.NewDiagnostic("executions.hub")},
				},
			)

			// The private session subscribed to the account execution stream
			// during its own boot (before the desk existed), so its account
			// subscription was gated on this workload. Now that the executions
			// ingress is wired, release the gate so execution frames can flow.
			privateSession.MarkExecutionsReady()

			futuresIngress["ticker"] = nmruntime.NewWorkload(
				cmd.Context(),
				append(
					[][]nmruntime.Node[*types.Envelope]{
						{system.NewDiagnostic("futures.ticker.ingress")},
						{system.NewTraced("futures.ticker.derivatives", derivatives.NewSignal(cmd.Context()))},
					},
					append(
						semanticCoreLight("futures.ticker", advisors, categorySolver),
						[][]nmruntime.Node[*types.Envelope]{
							{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
							{hub},
							{system.NewDiagnostic("futures.ticker.hub")},
						}...,
					)...,
				),
			)

			futuresIngress["trade"] = nmruntime.NewWorkload(
				cmd.Context(),
				append(
					[][]nmruntime.Node[*types.Envelope]{
						{system.NewDiagnostic("futures.trade.ingress")},
						{system.NewTraced("futures.trade.derivatives", derivatives.NewSignal(cmd.Context()))},
					},
					append(
						semanticCoreLight("futures.trade", advisors, categorySolver),
						[][]nmruntime.Node[*types.Envelope]{
							{witnessNode{asyncWriter: asyncWitness, writer: rawCapture, categorySolver: categorySolver, graphSolver: graphSolver}},
							{hub},
							{system.NewDiagnostic("futures.trade.hub")},
						}...,
					)...,
				),
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
