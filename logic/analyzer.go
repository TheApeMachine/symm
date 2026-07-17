package logic

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the composed analysis responsibilities after every signal
has measured the current Thesis. The manifold solver owns the Hawkes-driven GPU
field step, while the Analyzer builds each symbol's typed evidence topology.
*/
type Analyzer struct {
	ctx       context.Context
	gate      stageGate
	status    types.Status
	manifold  *manifold.Solver
	hawkes    manifold.HawkesSource
	tree      *dmt.Tree
	ui        chan []byte
	recorder  *audit.Recorder
	resonance map[string]*Resonance
	causal    map[string]*Causal
	rem       *remSleep
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
	if analyzer == nil {
		return
	}

	analyzer.recorder = recorder

	if analyzer.manifold != nil {
		analyzer.manifold.SetRecorder(recorder)
	}

	if analyzer.rem != nil {
		analyzer.rem.SetRecorder(recorder)
	}
}

/*
publish sends one small datura frame to the UI the moment the analyzer has the
evidence, mirroring broker.Balance.Publish.
*/
func (analyzer *Analyzer) publish(frame datura.Map[any]) {
	select {
	case analyzer.ui <- frame.Marshal():
	default:
	}
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
) (*Analyzer, error) {
	solver, err := manifold.NewSolver(api)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "logic analyzer: manifold init failed", err)
	}

	return &Analyzer{
		ctx:       ctx,
		gate:      gate,
		status:    types.READY,
		manifold:  solver,
		hawkes:    hawkes,
		tree:      tree,
		ui:        ui,
		resonance: make(map[string]*Resonance),
		causal:    make(map[string]*Causal),
		rem:       newREMSleep(ctx, tree),
	}, nil
}

/*
Initialize marks the analyzer ready for thesis updates.
*/
func (analyzer *Analyzer) Initialize() error {
	errnie.Info("initializing analyzer")
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
func (analyzer *Analyzer) Close() {
	if analyzer.rem != nil {
		analyzer.rem.Close()
	}

	if analyzer.manifold != nil {
		analyzer.manifold.Close()
	}
}

/*
Update delegates Hawkes-driven field analysis after signal measure, then composes
the current typed relationships for each symbol's evidence graph.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	started := time.Now()

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "analyze_begin", nil))
	analyzer.stepManifold(thesis)
	states := analyzer.observeStates(thesis)
	analyzer.publishMeasured(thesis, states)
	analyzer.composeGraphs(thesis)
	remObservations, remRequested := analyzer.cognizeStates(thesis, states)
	analyzer.consolidate(thesis, remObservations, remRequested)
	analyzer.publishCognition(thesis)
	analyzer.finish(thesis, states, started)
}

/*
finish audits analyze_end with the terminal rem and forecast counts.
*/
func (analyzer *Analyzer) finish(
	thesis *types.Thesis,
	states []manifold.State,
	started time.Time,
) {
	remPending := 0

	if analyzer.rem != nil {
		remPending = analyzer.rem.Pending()
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "analyze_end", map[string]any{
		"ns":          time.Since(started).Nanoseconds(),
		"states":      len(states),
		"forecasts":   len(thesis.Forecasts),
		"hypotheses":  len(thesis.Hypotheses),
		"rem_pending": remPending,
	}))
}
