package logic

import (
	"context"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/types"
)

type Solver interface {
	Update(thesis *types.Thesis) error
	Close() error
}

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
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	tree      *dmt.Tree
	manifold  *manifold.Solver
	resonance *resonance.Solver
	causal    *causal.Solver
	cognition *cognition.Solver
	graph     *graph.Solver
	ui        chan []byte
	binui     chan []byte
	recorder  *audit.Recorder
	thesis    *types.Thesis
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
	api *websocket.API,
	tree *dmt.Tree,
	ui chan []byte,
	binui chan []byte,
	recorder *audit.Recorder,
) (*Analyzer, error) {
	ctx, cancel := context.WithCancel(ctx)

	analyzer := &Analyzer{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		tree:      tree,
		manifold:  manifold.NewSolver(ui, binui, recorder),
		resonance: resonance.NewSolver(ui, recorder),
		causal:    causal.NewSolver(ui, recorder),
		cognition: cognition.NewSolver(tree, ui, recorder),
		graph:     graph.NewSolver(recorder),
		ui:        ui,
		recorder:  recorder,
	}

	// ticker and trade come from Hawkes cuts at depth one for manifold.
	// thesis comes from every other signal's SignalResult for categories+cognition.
	analyzer.Actor = types.NewActor(ctx, "analyzer", map[string]types.Handler{
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

func (analyzer *Analyzer) onSignal(message any) any {
	thesis, ok := message.(*types.Thesis)

	if !ok {
		return nil
	}

	for _, solver := range []Solver{
		analyzer.manifold,
		analyzer.resonance,
		analyzer.causal,
		analyzer.cognition,
		analyzer.graph,
	} {
		if err := solver.Update(thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed manifold step",
				err,
			))

			return nil
		}
	}

	return thesis
}

/*
Status reports analyzer readiness for the boot gate.
*/
func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
}

/*
Close the analyzer and all its solvers.
*/
func (analyzer *Analyzer) Close() error {
	analyzer.cancel()

	for _, solver := range []Solver{
		analyzer.manifold,
		analyzer.resonance,
		analyzer.causal,
		analyzer.cognition,
		analyzer.graph,
	} {
		if err := solver.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to close solver",
				err,
			))
		}
	}

	return nil
}
