package logic

import (
	"context"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the composed analysis responsibilities as signals stream
their measurements onto the shared Thesis. Every signal that publishes new
measurements triggers category composition, cognition, and UI publish. The
Hawkes signal additionally drives the manifold solver through its ticker/trade
cuts at depth one so the shared field steps on each excitation advance.
The thesis field is the shared boot pointer, set by Initialize so onSignal can
snapshot fresh measurements and run the analysis pipeline.
*/
type Analyzer struct {
	*types.Actor
	ctx           context.Context
	cancel        context.CancelFunc
	gate          stageGate
	status        types.Status
	thesis        *types.Thesis
	manifold      *manifold.Solver
	hawkes        manifold.HawkesSource
	tree          *dmt.Tree
	ui            chan []byte
	recorder      *audit.Recorder
	resonance     map[string]*Resonance
	causal        map[string]*Causal
	cognition     map[string]types.Cognition
	cognitionPath []string
	cognitionLast types.CategoryType
	observed      map[string]uint64
	rem           *remSleep
	categories    *category.Graph
	frameRows     []any
	cogRows       []types.Cognition
	catRows       []types.Category
	hypRows       []types.Hypothesis
	measureRows   []*types.Measurement
	bestBySym     map[string]types.Category
	stateRows     []manifold.State
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
		bestBySym:  make(map[string]types.Category),
	}

	// ticker and trade come from Hawkes cuts at depth one for manifold.
	// thesis comes from every other signal's SignalResult for categories+cognition.
	analyzer.Actor = types.NewActor(ctx, "analyzer", map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: analyzer.onHawkesCut},
		"trade":  {Topic: "trade", Fn: analyzer.onHawkesCut},
		"thesis": {Topic: "thesis", Fn: analyzer.onSignal},
	})
	analyzer.SetRecorder(recorder)

	return analyzer, nil
}

/*
Initialize attaches analyzer to upstream topics. Hawkes ticker/trade are wired
at depth one so each cut is processed against its Outcome snapshot. Every
non-Hawkes signal's thesis topic is wired so any signal publish triggers
category+cognition analysis. The thesis pointer is the shared boot pointer
set by the caller via SetThesis.
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

/*
onSignal runs the analysis pipeline when any non-Hawkes signal publishes new
measurements: category composition, cognize, publish UI frames. The manifold
step is skipped — that only runs on Hawkes cuts which arrive via ticker/trade.
*/
func (analyzer *Analyzer) onSignal(message any) any {
	thesis := analyzer.thesisFromSignal(message)

	if thesis == nil {
		return nil
	}

	analyzer.stampAndBegin(thesis, 0, 0)

	var composeStarted time.Time

	if analyzer.recorder != nil {
		composeStarted = time.Now()
	}

	analyzer.composeCategories(thesis)

	if analyzer.recorder != nil {
		errnie.Error(audit.Phase(
			analyzer.recorder, thesis.Tick, "categories_compose",
			map[string]any{"ns": time.Since(composeStarted).Nanoseconds(), "categories": len(thesis.Categories)},
		))
	}

	// publishMeasured with no manifold states (only resonance/causal/hypotheses
	// from prior Hawkes cuts; signal-only cuts do not have fresh states).
	analyzer.publishMeasured(thesis, nil, 0, thesis.Tick)

	remObservations, remRequested := analyzer.cognizeStates(thesis, nil, 0, thesis.Tick)

	var commitStarted time.Time

	if analyzer.recorder != nil {
		commitStarted = time.Now()
	}

	analyzer.categories.UpdateFrom(thesis)

	if analyzer.recorder != nil {
		errnie.Error(audit.Phase(
			analyzer.recorder, thesis.Tick, "categories_commit",
			map[string]any{"ns": time.Since(commitStarted).Nanoseconds(), "categories": len(thesis.Categories)},
		))
	}

	analyzer.consolidate(thesis, remObservations, remRequested)
	analyzer.publishCognition(thesis)

	payload := map[string]any{
		"ns":         int64(0),
		"forecasts":  len(thesis.Forecasts),
		"hypotheses": len(thesis.Hypotheses),
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "analyze_end", payload))

	return thesis
}

/*
thesisFromSignal returns the analyzer's stored shared Thesis pointer. All
signals publish onto the same Thesis, so the pointer is set once at Initialize.
*/
func (analyzer *Analyzer) thesisFromSignal(_ any) *types.Thesis {
	return analyzer.thesis
}

/*
onHawkesCut processes a Hawkes ticker/trade cut through the full pipeline
including manifold step, resonance, causal, categories, and cognition.
*/
func (analyzer *Analyzer) onHawkesCut(message any) any {
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

	if cut, ok := message.(*hawkes.Cut); ok {
		thesis := cut.SharedThesis()

		if thesis == nil {
			return nil, cut, 0, 0
		}

		return thesis, cut, 0, thesis.Tick
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
enrichCut runs full analysis including manifold for Hawkes-driven cuts.
*/
func (analyzer *Analyzer) enrichCut(
	thesis *types.Thesis,
	hawkes manifold.HawkesSource,
	cutID types.CutID,
	tick int64,
) {
	started := time.Now()
	analyzer.stamp(thesis)

	if analyzer.recorder != nil {
		payload := map[string]any{}
		if cutID > 0 {
			payload["cut_id"] = uint64(cutID)
		}
		errnie.Error(audit.Phase(analyzer.recorder, tick, "analyze_begin", payload))
	}

	analyzer.stepManifold(thesis, hawkes, cutID, tick)
	states := analyzer.observeStates(thesis, cutID, tick)

	var composeStarted time.Time
	if analyzer.recorder != nil {
		composeStarted = time.Now()
	}

	analyzer.composeCategories(thesis)

	if analyzer.recorder != nil {
		payload := map[string]any{
			"ns":         time.Since(composeStarted).Nanoseconds(),
			"categories": len(thesis.Categories),
		}
		if cutID > 0 {
			payload["cut_id"] = uint64(cutID)
		}
		errnie.Error(audit.Phase(analyzer.recorder, tick, "categories_compose", payload))
	}

	analyzer.publishMeasured(thesis, states, cutID, tick)

	remObservations, remRequested := analyzer.cognizeStates(thesis, states, cutID, tick)

	var commitStarted time.Time
	if analyzer.recorder != nil {
		commitStarted = time.Now()
	}

	analyzer.categories.UpdateFrom(thesis)

	if analyzer.recorder != nil {
		payload := map[string]any{
			"ns":         time.Since(commitStarted).Nanoseconds(),
			"categories": len(thesis.Categories),
		}
		if cutID > 0 {
			payload["cut_id"] = uint64(cutID)
		}
		errnie.Error(audit.Phase(analyzer.recorder, tick, "categories_commit", payload))
	}

	analyzer.consolidate(thesis, remObservations, remRequested)
	analyzer.publishCognition(thesis)

	analyzer.finish(thesis, states, started, cutID, tick)
}

/*
composeCategories rebuilds Thesis category rows from measurements × affinity and
publishes the resident graph pointer for strategy. Edge/prior mutation waits
until after cognize so DMT still sees the previous top for transition tokens.
The measurements slice is a pre-snapshotted copy shared with commitCategories.
*/
func (analyzer *Analyzer) composeCategories(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	if analyzer.categories == nil {
		analyzer.categories = category.NewGraph()
	}

	if thesis.Categories == nil {
		thesis.Categories = make(map[string][]types.Category)
	}

	for symbol, rows := range thesis.Categories {
		thesis.Categories[symbol] = rows[:0]
	}

	thesis.EachMeasurement(func(measurement *types.Measurement) bool {
		analyzer.composeMeasurement(thesis, measurement)
		return true
	})

	for symbol, rows := range thesis.Categories {
		if len(rows) == 0 {
			delete(thesis.Categories, symbol)
		}
	}

	thesis.Graphs.Store("categories", analyzer.categories)
}

/*
composeMeasurement projects one published measurement directly into the Thesis
category bucket for its symbol. The Thesis map owns the reusable per-symbol
slice, so Analyzer no longer allocates a temporary grouped book before writing
the category surface consumed by graph, cognition, strategy, and UI.
*/
func (analyzer *Analyzer) composeMeasurement(
	thesis *types.Thesis,
	measurement *types.Measurement,
) {
	if measurement == nil || measurement.Symbol == "" ||
		measurement.Validity.State != types.ValidityValid ||
		len(measurement.Metrics) == 0 {
		return
	}

	measurement.EachMetric(func(
		metric types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
	) bool {
		affinity, ok := types.AffinityFor(metric)

		if !ok {
			return true
		}

		mass, ok := categoryMass(sample)

		if !ok {
			return true
		}

		for _, categoryType := range affinity.Supports {
			thesis.Categories[measurement.Symbol] = append(
				thesis.Categories[measurement.Symbol],
				types.Category{
					Symbol:      measurement.Symbol,
					Type:        categoryType,
					Confidence:  mass / (1 + categoryUncertainty(measurement)),
					Strength:    mass,
					Maturity:    measurement.Maturity,
					Uncertainty: categoryUncertainty(measurement),
					Freshness:   measurement.Maturity,
					Supporting:  []string{string(metric)},
				},
			)
		}

		return true
	})
}

/*
categoryMass returns the absolute normalized metric mass when available, or raw
mass otherwise. Category rows are direct evidence projections, so no local
accumulator rescales the signal's own normalization before it reaches Thesis.
*/
func categoryMass(sample types.MetricSample) (float64, bool) {
	if sample.Normalized != nil {
		mass := math.Abs(*sample.Normalized)

		return mass, mass > 0 && !math.IsNaN(mass) && !math.IsInf(mass, 0)
	}

	mass := math.Abs(sample.Raw)

	return mass, mass > 0 && !math.IsNaN(mass) && !math.IsInf(mass, 0)
}

/*
categoryUncertainty reads a measurement interval width as category uncertainty
without creating an intermediate accumulator for the whole symbol.
*/
func categoryUncertainty(measurement *types.Measurement) float64 {
	if measurement == nil || measurement.Uncertainty == nil {
		return 0
	}

	return math.Abs(measurement.Uncertainty.Upper-measurement.Uncertainty.Lower) / 2
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
SetThesis stores the shared boot Thesis pointer so onSignal can snapshot
measurements and run the analysis pipeline without receiving the pointer
through every message.
*/
func (analyzer *Analyzer) SetThesis(thesis *types.Thesis) {
	analyzer.thesis = thesis
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
	if analyzer.ui == nil || len(frame) == 0 {
		frame.Free()
		return
	}

	analyzer.publishBytes(frame.MarshalAndFree())
}

/*
publishBytes enqueues one already encoded UI frame without blocking analysis.
Binary manifold textures use this path so they follow the same saturation
contract as JSON frames.
*/
func (analyzer *Analyzer) publishBytes(frame []byte) {
	if analyzer.ui == nil || len(frame) == 0 {
		return
	}

	select {
	case analyzer.ui <- frame:
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"logic analyzer: ui channel saturated; dropped frame",
			nil,
		))
	}
}

/*
stampAndBegin audits the analysis start for a non-Hawkes signal cut.
*/
func (analyzer *Analyzer) stampAndBegin(
	thesis *types.Thesis,
	cutID types.CutID,
	tick int64,
) {
	analyzer.stamp(thesis)

	payload := map[string]any{}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "analyze_begin", payload))
}

/*
stamp advances Thesis.At from the already-snapshotted measurement rows. This is
the analyzer hot path equivalent of Thesis.StampAt without paying for another
pointer-slice copy before category composition.
*/
func (analyzer *Analyzer) stamp(
	thesis *types.Thesis,
) {
	thesis.EachMeasurement(func(measurement *types.Measurement) bool {
		if measurement != nil && measurement.At.After(thesis.At) {
			thesis.At = measurement.At
		}

		return true
	})
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
	payload := map[string]any{
		"ns":          time.Since(started).Nanoseconds(),
		"states":      len(states),
		"forecasts":   len(thesis.Forecasts),
		"hypotheses":  len(thesis.Hypotheses),
		"rem_pending": analyzer.rem.Pending(),
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "analyze_end", payload))
}
