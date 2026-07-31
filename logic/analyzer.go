package logic

import (
	"context"
	"sync"

	"github.com/spf13/viper"
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
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	thesesIn  chan *types.Thesis
	tree      *dmt.Tree
	manifold  *manifold.Solver
	resonance *resonance.Solver
	causal    *causal.Solver
	cognition *cognition.Solver
	graph     *graph.Solver
	ui        chan []byte
	binui     chan []byte
	recorder  *audit.Recorder
	subMu     sync.Mutex
	thesesOut []*types.Subscription[*types.Thesis]
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
) *Analyzer {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	analyzer := &Analyzer{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		thesesIn:  make(chan *types.Thesis, buffer),
		tree:      tree,
		manifold:  manifold.NewSolver(api, ui, binui, recorder),
		resonance: resonance.NewSolver(ui, recorder),
		causal:    causal.NewSolver(ui, recorder),
		cognition: cognition.NewSolver(tree, ui, recorder),
		graph:     graph.NewSolver(recorder),
		ui:        ui,
		recorder:  recorder,
	}

	return analyzer
}

/*
Initialize attaches analyzer to upstream thesis subscriptions and starts the
direct channel loop that feeds Planner.
*/
func (analyzer *Analyzer) Initialize(signals ...*types.Subscription[*types.Thesis]) error {
	errnie.Info("initializing analyzer")
	go analyzer.run()

	if len(signals) == 0 {
		analyzer.status = types.READY
		return nil
	}

	for _, signal := range signals {
		if signal == nil {
			continue
		}

		go analyzer.forward(signal)
	}

	analyzer.status = types.READY

	return nil
}

func (analyzer *Analyzer) Thesis() *types.Subscription[*types.Thesis] {
	subscription := types.NewSubscription[*types.Thesis]()
	analyzer.subMu.Lock()
	analyzer.thesesOut = append(analyzer.thesesOut, subscription)
	analyzer.subMu.Unlock()
	return subscription

}

func (analyzer *Analyzer) Enqueue(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	select {
	case <-analyzer.ctx.Done():
		return
	case analyzer.thesesIn <- thesis:
	}
}

func (analyzer *Analyzer) forward(signal *types.Subscription[*types.Thesis]) {
	for {
		select {
		case <-analyzer.ctx.Done():
			return
		case thesis := <-signal.Channel:
			analyzer.Enqueue(thesis)
		}
	}
}

func (analyzer *Analyzer) run() {
	for {
		select {
		case <-analyzer.ctx.Done():
			return
		case thesis := <-analyzer.thesesIn:
			analyzer.onSignal(thesis)
		}
	}
}

func (analyzer *Analyzer) onSignal(thesis *types.Thesis) {
	if thesis == nil {
		return
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

			return
		}
	}

	analyzer.subMu.Lock()
	subscribers := append([]*types.Subscription[*types.Thesis](nil), analyzer.thesesOut...)
	analyzer.subMu.Unlock()

	for _, subscription := range subscribers {
		subscription.Send(thesis)
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
