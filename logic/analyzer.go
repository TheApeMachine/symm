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
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer is the entrypoint to the logic stage. This stage is responsible for
part dimensionality reduction of the Measurements (the Signal outputs), and
part an enrichment of the Metrics that the Measurements carry. Finally, the
logic stage is also the final structural transformation, which completes the
full transformation of raw market data to something that can be used by the
Strategy package to make Decisions. The final output of the Logic stage is
the Graph, which should encode everything that has been collected so far.
*/
type Analyzer struct {
	*types.Actor
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	thesis      *types.Thesis
	manifold    *manifold.Solver
	hawkes      manifold.HawkesSource
	tree        *dmt.Tree
	ui          chan []byte
	recorder    *audit.Recorder
	resonance   map[string]*Resonance
	causal      map[string]*Causal
	cognition   map[string]types.Cognition
	observed    map[string]uint64
	rem         *remSleep
	categories  *category.Graph
	frameRows   []any
	cogRows     []types.Cognition
	catRows     []types.Category
	hypRows     []types.Hypothesis
	measureRows []*types.Measurement
	bestBySym   map[string]types.Category
	stateRows   []manifold.State
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
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
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		manifold:  solver,
		hawkes:    hawkes,
		tree:      tree,
		ui:        ui,
		recorder:  recorder,
		resonance: make(map[string]*Resonance),
		causal:    make(map[string]*Causal),
		cognition: make(map[string]types.Cognition),
		observed:  make(map[string]uint64),
		rem:       newREMSleep(ctx, tree),
		bestBySym: make(map[string]types.Category),
	}

	// ticker and trade come from Hawkes cuts at depth one for manifold.
	// thesis comes from every other signal's SignalResult for categories+cognition.
	analyzer.Actor = types.NewActor(ctx, "analyzer", map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: analyzer.onHawkesCut},
		"trade":  {Topic: "trade", Fn: analyzer.onHawkesCut},
		"thesis": {Topic: "thesis", Fn: analyzer.onSignal},
	})

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
Update runs Hawkes-driven field analysis after signal measure: manifold step,
category composition, observation, publish, cognition, and finish.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	analyzer.enrich(thesis, analyzer.hawkes)
}

/*
Publish keeps the external analyzer UI publish entrypoint while delegating to
the internal non-blocking JSON frame sender.
*/
func (analyzer *Analyzer) Publish(frame datura.Map[any]) {
	analyzer.publish(frame)
}

/*
publish enqueues one JSON UI frame without blocking analysis so hot-path logic
can shed UI load instead of stalling market processing.
*/
func (analyzer *Analyzer) publish(frame datura.Map[any]) {
	select {
	case analyzer.ui <- frame.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"logic analyzer: ui channel saturated; dropped frame",
			nil,
		))
	}
}

/*
publishBytes enqueues one already encoded UI frame without blocking analysis.
Binary manifold textures use this path so they follow the same saturation
contract as JSON frames.
*/
func (analyzer *Analyzer) publishBytes(frame []byte) {
	if len(frame) == 0 {
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
