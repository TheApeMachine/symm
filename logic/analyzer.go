package logic

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
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
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	tree      *dmt.Tree
	cognition *cognition.Solver
	graph     *graph.Solver
	solvers   []Solver
	ui        chan []byte
	binui     chan types.FluidFrame
	recorder  *audit.Recorder
	thesis    *types.Thesis
	semaphore chan struct{}
}

/*
NewAnalyzer composes the field processor required by the analysis stage.
*/
func NewAnalyzer(
	ctx context.Context,
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
		solvers: []Solver{
			category.NewSolver(api, ui, recorder),
			manifold.NewSolver(api, ui, binui, recorder),
			resonance.NewSolver(
				ctx,
				ui,
				recorder,
				viper.GetFloat64("resonance.learning_rate"),
			),
			causal.NewSolver(ui, recorder),
			cognition.NewSolver(tree, ui, recorder),
			graph.NewSolver(ui, recorder),
		},
		ui:        ui,
		binui:     binui,
		recorder:  recorder,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
	}

	analyzer.thesis.Subscribe(types.SourceCategories, analyzer.semaphore)
	analyzer.run()
	return analyzer
}

func (analyzer *Analyzer) run() {
	go func() {
		for {
			select {
			case <-analyzer.ctx.Done():
				return
			case <-analyzer.semaphore:
				analyzer.process(analyzer.thesis)
			}
		}
	}()
}

func (analyzer *Analyzer) process(in any) {
	thesis, ok := in.(*types.Thesis)

	if ok {
		// Each solver reads outputs stamped by its predecessors. Running this chain
		// concurrently mixes prior-epoch readiness with current-epoch values.
		for _, solver := range analyzer.solvers {
			if err := solver.Update(thesis); err != nil {
				errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to update analyzer: "+err.Error(),
					err,
				))

				continue
			}
		}
	}

	thesis.Fanout(types.SourceCategories)
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
