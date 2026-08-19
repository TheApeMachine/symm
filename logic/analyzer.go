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
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

type Solver interface {
	Name() string
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
	ctx          context.Context
	cancel       context.CancelFunc
	status       types.Status
	tree         *dmt.Tree
	solverGroups [][]Solver
	ui           chan []byte
	binui        chan types.FluidFrame
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
	ui chan []byte,
	binui chan types.FluidFrame,
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
				category.NewSolver(api, ui, recorder),
				manifold.NewSolver(api, ui, binui, recorder),
			}, {
				causal.NewSolver(price, ui, recorder),
				cognition.NewSolver(tree, ui, recorder),
			}, {
				graph.NewSolver(ui, recorder),
			}},
		ui:       ui,
		binui:    binui,
		recorder: recorder,
	}

	return analyzer
}

func (analyzer *Analyzer) Process(thesis *types.Thesis) error {
	previousGroupEnd := time.Now()

	for groupIndex, solvers := range analyzer.solverGroups {
		groupStarted := time.Now()

		if analyzer.ObserveHop != nil && groupIndex > 0 && len(solvers) > 0 {
			previous := analyzer.solverGroups[groupIndex-1][0].Name()
			analyzer.ObserveHop(previous, solvers[0].Name(), groupStarted.Sub(previousGroupEnd))
		}

		group, ctx := errgroup.WithContext(analyzer.ctx)

		for _, solver := range solvers {
			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				solverStarted := time.Now()
				err := solver.Update(thesis)

				if analyzer.ObserveModule != nil {
					analyzer.ObserveModule(solver.Name(), time.Since(solverStarted))
				}

				return err
			})
		}

		if err := group.Wait(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"analyzer: parallel solver update failed",
				err,
			))
		}

		previousGroupEnd = time.Now()
	}

	return nil
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
func (analyzer *Analyzer) Close() (err error) {
	analyzer.cancel()

	for _, group := range analyzer.solverGroups {
		for _, solver := range group {
			err = errors.Join(err, errnie.Error(solver.Close()))
		}
	}

	return err
}
