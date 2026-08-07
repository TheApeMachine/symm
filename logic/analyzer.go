package logic

import (
	"context"
	"sync"

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
	"github.com/theapemachine/symm/utils"
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
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	tree          *dmt.Tree
	cognition     *cognition.Solver
	graph         *graph.Solver
	solvers       []Solver
	ui            chan []byte
	binui         chan []byte
	recorder      *audit.Recorder
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
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
	subscriptions map[string]*types.Subscription[any],
) *Analyzer {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	analyzer := &Analyzer{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		tree:   tree,
		solvers: []Solver{
			category.NewSolver(api, ui, recorder),
			manifold.NewSolver(api, ui, binui, recorder),
			resonance.NewSolver(ui, recorder),
			causal.NewSolver(ui, recorder),
			cognition.NewSolver(tree, ui, recorder),
			graph.NewSolver(ui, recorder),
		},
		ui:            ui,
		binui:         binui,
		recorder:      recorder,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	analyzer.run()
	return analyzer
}

func (analyzer *Analyzer) run() {
	go func() {
		for {
			select {
			case <-analyzer.ctx.Done():
				return
			case in := <-analyzer.subscriptions["correlation"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["cvd"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["depthflow"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["exhaustion"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["hawkes"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["leadlag"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["liquidity"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["pumpdump"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["sentiment"].Channel:
				analyzer.process(in)
			case in := <-analyzer.subscriptions["toxicity"].Channel:
				analyzer.process(in)
			}
		}
	}()
}

func (analyzer *Analyzer) process(in any) {
	thesis, ok := in.(*types.Thesis)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"failed to convert signal payload to thesis",
			nil,
		))

		return
	}

	if thesis.Readiness.SignalsMeasured() {
		// A pond task group is cancelled permanently after one task returns an
		// error. Each Thesis pass needs its own group so an earlier failed solver
		// does not prevent every solver from running on later market updates.
		group, _ := errgroup.WithContext(analyzer.ctx)

		// A stage that fails is recorded and the pass continues. Readiness is
		// taken from the stamps each stage leaves, so one that did not complete
		// simply never stamps and the planner declines the tick on its own;
		// returning here would instead discard the work of every stage that did
		// run, including the ones that had already finished.
		for _, solver := range analyzer.solvers {
			group.Go(func() error {
				return solver.Update(thesis)
			})
		}

		if err := group.Wait(); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to update analyzers: "+err.Error(),
				err,
			))

		}
	}

	utils.Fanout(
		analyzer.subscribers,
		"analyzer",
		thesis,
	)
}

func (analyzer *Analyzer) Subscribe(
	key string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
		analyzer.subscribers,
		key,
		subscription,
	)
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
