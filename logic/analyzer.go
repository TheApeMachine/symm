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
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	tree      *dmt.Tree
	cognition *cognition.Solver
	graph     *graph.Solver
	manifold  *manifold.Solver
	solvers   []Solver
	ui        chan []byte
	binui     chan types.FluidFrame
	recorder  *audit.Recorder
	thesis    *types.Thesis
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
	manifoldSolver := manifold.NewSolver(api, ui, binui, recorder)

	analyzer := &Analyzer{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		tree:   tree,
		solvers: []Solver{
			category.NewSolver(api, ui, recorder),
			resonance.NewSolver(
				ctx,
				ui,
				recorder,
				viper.GetFloat64("resonance.learning_rate"),
			),
			manifoldSolver,
			causal.NewSolver(price, ui, recorder),
			cognition.NewSolver(tree, ui, recorder),
			graph.NewSolver(ui, recorder),
		},
		manifold: manifoldSolver,
		ui:       ui,
		binui:    binui,
		recorder: recorder,
		thesis:   thesis,
	}

	return analyzer
}

func (analyzer *Analyzer) Process(thesis *types.Thesis) {
	iterationErr := ""

	for !thesis.SymbolsReady() {
		readinessRevision := thesis.ReadinessRevision()
		group := &errgroup.Group{}

		for _, solver := range analyzer.solvers {
			group.Go(func() error {
				return solver.Update(thesis)
			})
		}

		err := group.Wait()

		waiting := err == nil && analyzer.manifold != nil &&
			analyzer.manifold.WaitingForBook()
		stalled := err == nil && !waiting && !thesis.SymbolsReady() &&
			thesis.ReadinessRevision() == readinessRevision

		if err != nil {
			iterationErr = err.Error()
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to update analyzer: "+err.Error(),
				err,
			))
		}

		if stalled {
			iterationErr = "no solver advanced readiness"
			errnie.Error(errnie.Err(
				errnie.Internal,
				"analyzer: "+iterationErr,
				nil,
			))
		}

		if err != nil || waiting {
			return
		}

		if stalled {
			return
		}
	}
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
