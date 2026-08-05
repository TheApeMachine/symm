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
)

type Solver interface {
	Update(thesis *types.Thesis) error
	Close() error
}

/*
Evaluator turns an analyzed thesis into decisions and clears the evidence it
spent doing so. The analyzer runs it inline at the end of a pass, so ownership
of the thesis never leaves the goroutine that assembled it.
*/
type Evaluator interface {
	Update(thesis *types.Thesis) *types.Thesis
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
	evaluator     Evaluator
	mu            sync.Mutex
}

/*
AttachEvaluator registers the stage that consumes an analyzed thesis. It is
set after construction because the evaluator subscribes to the analyzer.
*/
func (analyzer *Analyzer) AttachEvaluator(evaluator Evaluator) {
	analyzer.mu.Lock()
	defer analyzer.mu.Unlock()

	analyzer.evaluator = evaluator
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

	/*
		Both are held as fields and run as solvers, so they are constructed
		once and shared rather than instantiated twice.
	*/
	cognitionSolver := cognition.NewSolver(tree, ui, recorder)
	graphSolver := graph.NewSolver(ui, recorder)

	analyzer := &Analyzer{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		tree:   tree,
		solvers: []Solver{
			/*
				Categories are the substrate the later stages reason over:
				the graph builds its nodes from them and cognition tokenizes
				them into sequences, so they are derived first.
			*/
			category.NewSolver(api, ui, recorder),
			manifold.NewSolver(api, ui, binui, recorder),
			resonance.NewSolver(ui, recorder),
			causal.NewSolver(ui, recorder),

			/*
				Cognition tokenizes categories into sequences and the graph
				builds its nodes from them, so both run after the stages
				they draw their evidence from.
			*/
			cognitionSolver,
			graphSolver,
		},
		cognition:     cognitionSolver,
		graph:         graphSolver,
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

	if thesis == nil || !thesis.Readiness.SignalsMeasured() {
		return
	}

	// A stage that fails is recorded and the pass continues. Readiness is
	// taken from the stamps each stage leaves, so one that did not complete
	// simply never stamps and the planner declines the tick on its own;
	// returning here would instead discard the work of every stage that did
	// run, including the ones that had already finished.
	for _, solver := range analyzer.solvers {
		if err := solver.Update(thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed logic solver",
				err,
			))
		}
	}

	if value, ok := analyzer.subscribers.Load("thesis"); ok {
		if subscribers, ok := value.([]*types.Subscription[any]); ok {
			for _, subscriber := range subscribers {
				subscriber.Send(thesis)
			}
		}
	}

	// Evaluate on this goroutine rather than handing the thesis to one of its
	// own. The evaluator prepares the next epoch's readiness and derived
	// snapshots, so those writes must remain ordered after this solver pass.
	if analyzer.evaluator != nil {
		analyzer.evaluator.Update(thesis)
	}
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
				"failed to close solver",
				err,
			))
		}
	}

	return nil
}
