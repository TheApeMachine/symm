package logic

import (
	"context"
	"time"

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
		solvers: []Solver{
			category.NewSolver(api, ui, recorder),
			resonance.NewSolver(
				ctx,
				ui,
				recorder,
				viper.GetFloat64("resonance.learning_rate"),
			),
			manifold.NewSolver(api, ui, binui, recorder),
			causal.NewSolver(price, ui, recorder),
			cognition.NewSolver(tree, ui, recorder),
			graph.NewSolver(ui, recorder),
		},
		ui:        ui,
		binui:     binui,
		recorder:  recorder,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
	}

	analyzer.thesis.Subscribe(types.SourceAnalyzer, analyzer.semaphore)

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

	if !ok || thesis == nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to process analyzer: thesis is nil",
			nil,
		))

		return
	}

	iterations := 0
	initiallyReady := thesis.SymbolsReady()

	if analyzer.recorder != nil {
		err := analyzer.recorder.Write(map[string]any{
			"channel": "orchestration",
			"type":    "analyzer_start",
			"value": map[string]any{
				"at":            time.Now().UTC(),
				"symbols_ready": initiallyReady,
			},
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"analyzer: failed to audit process start",
				err,
			))
		}
	}

	iterationErr := ""

	for !thesis.SymbolsReady() {
		group := &errgroup.Group{}

		for _, solver := range analyzer.solvers {
			group.Go(func() error {
				return solver.Update(thesis)
			})
		}

		if err := group.Wait(); err != nil {
			iterationErr = err.Error()
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to update analyzer: "+err.Error(),
				err,
			))
		}

		if analyzer.recorder != nil {
			err := analyzer.recorder.Write(map[string]any{
				"channel": "orchestration",
				"type":    "analyzer_iteration",
				"value": map[string]any{
					"at":            time.Now().UTC(),
					"iteration":     iterations,
					"symbols_ready": thesis.SymbolsReady(),
					"error":         iterationErr,
				},
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"analyzer: failed to audit process iteration",
					err,
				))
			}
		}

	}

	if analyzer.recorder != nil {
		err := analyzer.recorder.Write(map[string]any{
			"channel": "orchestration",
			"type":    "analyzer_complete",
			"value": map[string]any{
				"at":              time.Now().UTC(),
				"iterations":      iterations,
				"initially_ready": initiallyReady,
				"finally_ready":   thesis.SymbolsReady(),
			},
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"analyzer: failed to audit process completion",
				err,
			))
		}
	}

	thesis.Fanout(types.SourceAnalyzer, types.SourcePlanner)
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
