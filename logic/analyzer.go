package logic

import (
	"context"

	"github.com/spf13/viper"
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
	cognition    *cognition.Solver
	graph        *graph.Solver
	resonance    Solver
	solvers      []Solver
	solverGroups [][]Solver
	ui           chan []byte
	binui        chan types.FluidFrame
	recorder     *audit.Recorder
	thesis       *types.Thesis
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

func (analyzer *Analyzer) Process(
	thesis *types.Thesis,
	resonanceReady bool,
) error {
	for _, solvers := range analyzer.solverGroups {
		group, _ := errgroup.WithContext(analyzer.ctx)

		for _, solver := range solvers {
			if solver == analyzer.resonance && !resonanceReady {
				continue
			}

			group.Go(func() error {
				if err := solver.Update(thesis); err != nil {
					return errnie.Err(
						errnie.Internal,
						"analyzer: solver update failed",
						err,
					)
				}

				return nil
			})
		}

		if err := group.Wait(); err != nil {
			return err
		}
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
