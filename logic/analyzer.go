package logic

import (
	"context"
	"errors"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

type Solver interface {
	Name() string
	Error() error
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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	status       types.Status
	tree         *dmt.Tree
	solverGroups [][]Solver
	ui           *transport.MapReduce[*types.UIFrame]
	binui        *transport.MapReduce[types.FluidFrame]
	recorder     *audit.Recorder

	// ObserveModule / ObserveHop are optional diagnostics hooks. When set, the
	// analyzer reports each solver's work time and the wait between solver
	// groups so the wiring diagram can render per-system profiling and latency.
	ObserveModule func(string, time.Duration)
	ObserveHop    func(string, string, time.Duration)
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
	price *broker.Price,
	api *websocket.API,
	tree *dmt.Tree,
	ui *transport.MapReduce[*types.UIFrame],
	binui *transport.MapReduce[types.FluidFrame],
	recorder *audit.Recorder,
	thesis *types.Thesis,
) *Analyzer {
	ctx, cancel := context.WithCancel(ctx)

	analyzer := &Analyzer{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		tree:   tree,
		solverGroups: [][]Solver{
			{
				category.NewSolver(ctx, thesis, api, ui, recorder),
				manifold.NewSolver(ctx, thesis, api, ui, binui, recorder),
			}, {
				causal.NewSolver(thesis, price, ui, recorder),
				cognition.NewSolver(ctx, thesis, tree, ui, recorder),
			}, {
				graph.NewSolver(thesis, ui, recorder),
			}},
		ui:       ui,
		binui:    binui,
		recorder: recorder,
	}

	return analyzer
}

/*
Process confirms the analysis stage is ready. Run owns solver scheduling.
*/
func (analyzer *Analyzer) Process(_ *types.Thesis) error {
	return nil
}

func (analyzer *Analyzer) Name() string { return "analyzer" }

func (analyzer *Analyzer) Error() error { return analyzer.err }

/*
Status reports analyzer readiness for the boot gate.
*/
func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
}

/*
Close the analyzer and all its solvers.
*/
func (analyzer *Analyzer) Close() (err error) {
	analyzer.cancel()

	for _, group := range analyzer.solverGroups {
		for _, solver := range group {
			err = errors.Join(err, errnie.Error(solver.Close()))
		}
	}

	return err
}
