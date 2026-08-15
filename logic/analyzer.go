package logic

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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
	"github.com/theapemachine/symm/logic/resonance"
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
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	tree          *dmt.Tree
	cognition     *cognition.Solver
	graph         *graph.Solver
	resonance     Solver
	solvers       []Solver
	solverGroups  [][]Solver
	ui            chan []byte
	binui         chan types.FluidFrame
	recorder      *audit.Recorder
	thesis        *types.Thesis
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

	categorySolver := category.NewSolver(api, ui, recorder)
	resonanceSolver := resonance.NewSolver(
		ctx,
		ui,
		recorder,
		viper.GetFloat64("resonance.learning_rate"),
	)
	manifoldSolver := manifold.NewSolver(api, ui, binui, recorder)
	causalSolver := causal.NewSolver(price, ui, recorder)
	cognitionSolver := cognition.NewSolver(tree, ui, recorder)
	graphSolver := graph.NewSolver(ui, recorder)

	analyzer := &Analyzer{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		tree:      tree,
		resonance: resonanceSolver,
		solvers: []Solver{
			categorySolver,
			resonanceSolver,
			manifoldSolver,
			causalSolver,
			cognitionSolver,
			graphSolver,
		},
		solverGroups: [][]Solver{
			{categorySolver, resonanceSolver, manifoldSolver},
			{causalSolver, cognitionSolver},
			{graphSolver},
		},
		ui:       ui,
		binui:    binui,
		recorder: recorder,
		thesis:   thesis,
	}

	return analyzer
}

func (analyzer *Analyzer) Process(thesis *types.Thesis) error {
	groupNames := []string{
		string(types.SourceCategory),
		string(types.SourceCausal),
		string(types.SourceGraph),
	}

	previousEnd := time.Now()

	for groupIndex, solvers := range analyzer.solverGroups {
		if groupIndex > 0 && analyzer.ObserveHop != nil {
			analyzer.ObserveHop(
				groupNames[groupIndex-1],
				groupNames[groupIndex],
				time.Since(previousEnd),
			)
		}

		running := datura.NewMap()
		done := datura.NewMap()

		for _, solver := range solvers {
			running[solver.Name()] = "running"
			done[solver.Name()] = "done"
		}

		group, _ := errgroup.WithContext(analyzer.ctx)

		for _, solver := range solvers {
			solver := solver

			group.Go(func() error {
				started := time.Now()
				err := solver.Update(thesis)

				if analyzer.ObserveModule != nil {
					analyzer.ObserveModule(solver.Name(), time.Since(started))
				}

				if err != nil {
					return errnie.Err(
						errnie.Internal,
						"analyzer: solver update failed",
						err,
					)
				}

				return nil
			})
		}

		waitErr := group.Wait()

		if waitErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"analyzer: parallel solver update failed",
				waitErr,
			))
		}

		previousEnd = time.Now()
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
Settling reports whether any solver still holds work started by the last
Process call. The manifold is the only solver that continues after Update
returns.
*/
func (analyzer *Analyzer) Settling() bool {
	if analyzer == nil {
		return false
	}

	for _, solver := range analyzer.solvers {
		manifold, ok := solver.(*manifold.Solver)

		if !ok {
			continue
		}

		if manifold.Settling() {
			return true
		}
	}

	return false
}

/*
Close the analyzer and all its solvers.
*/
func (analyzer *Analyzer) Close() error {
	analyzer.cancel()

	for _, solver := range analyzer.solvers {
		if solver == nil {
			continue
		}

		if err := solver.Close(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to close solver: "+err.Error(),
				err,
			))
		}
	}

	return nil
}
