package logic

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the composed analysis responsibilities after every signal
has measured the current Thesis. The manifold solver owns the Hawkes-driven GPU
step over the shared market population, while the Analyzer builds each symbol's
typed evidence topology.
*/
type Analyzer struct {
	*types.Actor
	ctx        context.Context
	cancel     context.CancelFunc
	gate       stageGate
	status     types.Status
	manifold   *manifold.Solver
	hawkes     manifold.HawkesSource
	tree       *dmt.Tree
	ui         chan []byte
	recorder   *audit.Recorder
	resonance  map[string]*Resonance
	causal     map[string]*Causal
	cognition  map[string]types.Cognition
	observed   map[string]uint64
	rem        *remSleep
	categories *category.Graph
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
	gate stageGate,
	api *websocket.API,
	hawkes manifold.HawkesSource,
	tree *dmt.Tree,
	ui chan []byte,
	recorder *audit.Recorder,
) (*Analyzer, error) {
	solver, err := manifold.NewSolver(
		api,
		viper.GetInt("signals.feed_track_capacity"),
	)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "logic analyzer: manifold init failed", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	analyzer := &Analyzer{
		ctx:        ctx,
		cancel:     cancel,
		gate:       gate,
		status:     types.READY,
		manifold:   solver,
		hawkes:     hawkes,
		tree:       tree,
		ui:         ui,
		resonance:  make(map[string]*Resonance),
		causal:     make(map[string]*Causal),
		cognition:  make(map[string]types.Cognition),
		observed:   make(map[string]uint64),
		rem:        newREMSleep(ctx, tree),
		categories: category.NewGraph(),
	}

	analyzer.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: analyzer.thesis},
		"trade":  {Topic: "trade", Fn: analyzer.thesis},
	})
	analyzer.SetRecorder(recorder)

	return analyzer, nil
}

/*
Initialize attaches analyzer to Hawkes ticker/trade at depth one so each cut is
processed against its Outcome snapshot before the next trade mutates the live
Process, instead of coalescing many publishes onto the latest EventCount.
*/
func (analyzer *Analyzer) Initialize(signals ...types.Topic) error {
	errnie.Info("initializing analyzer")

	if len(signals) == 0 {
		analyzer.status = types.READY
		return nil
	}

	analyzer.Actor.InitializeSize(1, signals...)
	analyzer.status = types.READY

	return nil
}

/*
Status reports analyzer readiness for the boot gate.
*/
func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
}

/*
Close drains REM and releases the manifold solver.
*/
func (analyzer *Analyzer) Close() error {
	analyzer.cancel()

	if analyzer.rem != nil {
		analyzer.rem.Close()
	}

	if analyzer.manifold != nil {
		analyzer.manifold.Close()
	}

	return nil
}

func (analyzer *Analyzer) thesis(message any) any {
	thesis, source, cutID, tick := analyzer.bind(message)
	analyzer.enrichCut(thesis, source, cutID, tick)

	return thesis
}

/*
hawkesCut is a publish that carries both the shared Thesis and a frozen
HawkesSource so Analyzer does not reread the live Process after coalescing.
*/
type hawkesCut interface {
	SharedThesis() *types.Thesis
	manifold.HawkesSource
	CutIdentity() (types.CutID, int64)
}

/*
bind unwraps a cut frame or Hawkes cut into Thesis plus the Outcome snapshot.
Bare Thesis messages still use the live Hawkes source (unit paths).
*/
func (analyzer *Analyzer) bind(message any) (*types.Thesis, manifold.HawkesSource, types.CutID, int64) {
	if cut, ok := message.(hawkesCut); ok {
		cutID, tick := cut.CutIdentity()
		return cut.SharedThesis(), cut, cutID, tick
	}

	thesis := message.(*types.Thesis)
	return thesis, analyzer.hawkes, 0, thesis.Tick
}

/*
enrich runs analysis against an explicit HawkesSource for this cut.
*/
func (analyzer *Analyzer) enrich(thesis *types.Thesis, hawkes manifold.HawkesSource) {
	tick := int64(0)

	if thesis != nil {
		tick = thesis.Tick
	}

	analyzer.enrichCut(thesis, hawkes, 0, tick)
}

/*
enrichCut runs analysis against one cut identity so audit rows stay cut-true even
when the shared Thesis pointer advances before downstream handlers drain.
*/
func (analyzer *Analyzer) enrichCut(
	thesis *types.Thesis,
	hawkes manifold.HawkesSource,
	cutID types.CutID,
	tick int64,
) {
	started := time.Now()

	if thesis != nil {
		thesis.StampAt()
	}

	payload := map[string]any{}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "analyze_begin", payload))

	analyzer.stepManifold(thesis, hawkes, cutID, tick)
	states := analyzer.observeStates(thesis, cutID, tick)

	var composeStarted time.Time

	if analyzer.recorder != nil {
		composeStarted = time.Now()
	}

	analyzer.composeCategories(thesis)
	payload = map[string]any{
		"ns":         time.Since(composeStarted).Nanoseconds(),
		"categories": len(thesis.Categories),
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "categories_compose", payload))

	analyzer.publishMeasured(thesis, states, cutID, tick)

	remObservations, remRequested := analyzer.cognizeStates(thesis, states, cutID, tick)

	var commitStarted time.Time

	if analyzer.recorder != nil {
		commitStarted = time.Now()
	}

	analyzer.commitCategories(thesis)
	payload = map[string]any{
		"ns":         time.Since(commitStarted).Nanoseconds(),
		"categories": len(thesis.Categories),
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "categories_commit", payload))

	analyzer.consolidate(thesis, remObservations, remRequested)
	analyzer.publishCognition(thesis)

	analyzer.finish(thesis, states, started, cutID, tick)
}

/*
composeCategories rebuilds Thesis category rows from measurements × affinity and
publishes the resident graph pointer for strategy. Edge/prior mutation waits
until after cognize so DMT still sees the previous top for transition tokens.
*/
func (analyzer *Analyzer) composeCategories(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	if analyzer.categories == nil {
		analyzer.categories = category.NewGraph()
	}

	thesis.Categories = category.ComposeAll(thesis)
	thesis.Graphs.Store("categories", analyzer.categories)
}

/*
commitCategories strengthens resident graph edges from this cut's composed
categories and advances per-symbol priors after DMT has consumed transitions.
*/
func (analyzer *Analyzer) commitCategories(thesis *types.Thesis) {
	if thesis == nil || analyzer.categories == nil {
		return
	}

	analyzer.categories.Update(thesis.At, thesis, thesis.Categories)
}

/*
Update runs Hawkes-driven field analysis after signal measure: manifold step,
category composition, observation, publish, cognition, and finish.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	analyzer.enrich(thesis, analyzer.hawkes)
}

/*
stageGate exposes boot readiness without coupling Analyzer to boot orchestration.
*/
type stageGate interface {
	Ready(system.StageType) bool
}

/*
SetRecorder attaches the runtime audit stream to the analyzer, manifold solver,
and REM scheduler so phase breadcrumbs survive a freeze.
*/
func (analyzer *Analyzer) SetRecorder(recorder *audit.Recorder) {
	analyzer.recorder = recorder

	if analyzer.manifold != nil {
		analyzer.manifold.SetRecorder(recorder)
	}

	if analyzer.rem != nil {
		analyzer.rem.SetRecorder(recorder)
	}
}

/*
publish enqueues one frame for the UI hub. A full ingress drops the frame —
same contract as WireMeasurements — so dashboard backpressure cannot stall
enrich or the cut cascade behind Hawkes.
*/
func (analyzer *Analyzer) publish(frame datura.Map[any]) {
	payload, err := frame.Marshal()

	if err != nil {
		errnie.Error(err)
		return
	}

	analyzer.publishRaw(payload)
}

/*
publishRaw enqueues a pre-encoded UI payload (JSON or manifold binary lattice).
*/
func (analyzer *Analyzer) publishRaw(payload []byte) {
	if analyzer.ui == nil || len(payload) == 0 {
		return
	}

	select {
	case analyzer.ui <- payload:
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"logic analyzer: ui channel saturated; dropped frame",
			nil,
		))
	}
}

/*
finish audits analyze_end with the terminal rem and forecast counts.
*/
func (analyzer *Analyzer) finish(
	thesis *types.Thesis,
	states []manifold.State,
	started time.Time,
	cutID types.CutID,
	tick int64,
) {
	remPending := 0

	if analyzer.rem != nil {
		remPending = analyzer.rem.Pending()
	}

	payload := map[string]any{
		"ns":          time.Since(started).Nanoseconds(),
		"states":      len(states),
		"forecasts":   len(thesis.Forecasts),
		"hypotheses":  len(thesis.Hypotheses),
		"rem_pending": remPending,
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "analyze_end", payload))
}
