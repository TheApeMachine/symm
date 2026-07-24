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
	ctx       context.Context
	cancel    context.CancelFunc
	gate      stageGate
	status    types.Status
	manifold  *manifold.Solver
	hawkes    manifold.HawkesSource
	tree      *dmt.Tree
	ui        chan []byte
	recorder  *audit.Recorder
	resonance map[string]*Resonance
	causal    map[string]*Causal
	cognition map[string]types.Cognition
	observed  map[string]uint64
	rem       *remSleep
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
		ctx:       ctx,
		cancel:    cancel,
		gate:      gate,
		status:    types.READY,
		manifold:  solver,
		hawkes:    hawkes,
		tree:      tree,
		ui:        ui,
		resonance: make(map[string]*Resonance),
		causal:    make(map[string]*Causal),
		cognition: make(map[string]types.Cognition),
		observed:  make(map[string]uint64),
		rem:       newREMSleep(ctx, tree),
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
	thesis, source := analyzer.bind(message)
	analyzer.enrich(thesis, source)

	return thesis
}

/*
hawkesCut is a publish that carries both the shared Thesis and a frozen
HawkesSource so Analyzer does not reread the live Process after coalescing.
*/
type hawkesCut interface {
	SharedThesis() *types.Thesis
	manifold.HawkesSource
}

/*
bind unwraps a cut frame or Hawkes cut into Thesis plus the Outcome snapshot.
Bare Thesis messages still use the live Hawkes source (unit paths).
*/
func (analyzer *Analyzer) bind(message any) (*types.Thesis, manifold.HawkesSource) {
	if cut, ok := message.(hawkesCut); ok {
		return cut.SharedThesis(), cut
	}

	return message.(*types.Thesis), analyzer.hawkes
}

/*
enrich runs analysis against an explicit HawkesSource for this cut.
*/
func (analyzer *Analyzer) enrich(thesis *types.Thesis, hawkes manifold.HawkesSource) {
	started := time.Now()

	if thesis != nil {
		thesis.StampAt()
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "analyze_begin", nil))

	analyzer.stepManifold(thesis, hawkes)
	states := analyzer.observeStates(thesis)

	analyzer.publishMeasured(thesis, states)

	remObservations, remRequested := analyzer.cognizeStates(thesis, states)
	analyzer.consolidate(thesis, remObservations, remRequested)
	analyzer.publishCognition(thesis)

	analyzer.finish(thesis, states, started)
}

/*
Update runs Hawkes-driven field analysis after signal measure: manifold step,
observation, publish, cognition, and finish. Evidence-graph composition is
removed until the resident market graph is redesigned.
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
publish sends one small datura frame to the UI hub ingress. The send blocks
until the drain accepts it so resonance, manifold, and cognition frames are
never silently discarded under load — a full buffer stalls analysis honestly
instead of leaving the terminal painted as "waiting."
*/
func (analyzer *Analyzer) publish(frame datura.Map[any]) {
	payload, err := frame.Marshal()

	if err != nil {
		errnie.Error(err)
		return
	}

	if analyzer.ctx == nil {
		analyzer.ui <- payload
		return
	}

	select {
	case analyzer.ui <- payload:
	case <-analyzer.ctx.Done():
	}
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
