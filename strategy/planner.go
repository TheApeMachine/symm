package strategy

import (
	"context"
	"slices"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Planner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	ui          chan []byte
	subscribers *sync.Map
	api         *websocket.API
	desk        *broker.Desk
	price       *broker.Price
	balance     *broker.Balance
	recorder    *audit.Recorder
	evaluator   Evaluator
	arbiter     *Arbiter
	allocator   *Allocator
	Thesis      *types.Thesis
}

func NewPlanner(
	ctx context.Context,
	uiHub chan []byte,
	api *websocket.API,
	desk *broker.Desk,
	instrument *broker.Instrument,
	price *broker.Price,
	balance *broker.Balance,
	analyzer *logic.Analyzer,
	recorder *audit.Recorder,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	evaluator := NewEvaluator(desk, price, balance, recorder)
	arbiter := NewArbiter(desk)

	planner := &Planner{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.READY,
		ui:          uiHub,
		subscribers: &sync.Map{},
		api:         api,
		desk:        desk,
		price:       price,
		balance:     balance,
		recorder:    recorder,
		evaluator:   evaluator,
		arbiter:     arbiter,
		allocator:   NewAllocator(ctx, balance, instrument, price, desk),
	}

	if analyzer != nil {
		planner.AttachAnalyzer(analyzer)
	}

	return planner
}

/*
AttachAnalyzer registers the planner as the analyzer's evaluator.

The planner runs inline on the analyzer's goroutine rather than consuming a
thesis subscription of its own. Update ends a cycle by resetting the thesis,
and doing that concurrently with the analyzer's next pass would clear evidence
while it is still being written.
*/
func (planner *Planner) AttachAnalyzer(analyzer *logic.Analyzer) {
	if analyzer == nil {
		return
	}

	analyzer.AttachEvaluator(planner)
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

func (planner *Planner) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	subscribers, ok := planner.subscribers.LoadOrStore(
		key, []*types.Subscription[any]{subscription},
	)

	if ok {
		planner.subscribers.Store(key, append(
			subscribers.([]*types.Subscription[any]),
			subscription,
		))
	}

	return subscription
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) *types.Thesis {
	if thesis == nil {
		return nil
	}

	thesis.Tick++

	errnie.Error(audit.Phase(
		planner.recorder, thesis.Tick, "decide_begin", nil,
	))

	if thesis.Status != types.READY {
		planner.complete(thesis, false, "status_not_ready")

		return thesis
	}

	if !thesis.Readiness.Complete() {
		planner.complete(thesis, false, "logic_not_ready")
		return thesis
	}

	/*
		Candidates are produced, then funded, then arbitrated.

		Sizing has to settle before slots do, because arbitration is what closes
		working positions to make room. Displacing an incumbent for a challenger
		the wallet cannot actually fund leaves the exit standing on its own: the
		allocator rejects the challenger a moment later, and the desk has closed
		a position for a trade that never gets placed.
	*/
	planner.evaluator.ManageContinuation(thesis, planner.desk, planner.price)
	planner.evaluator.EvaluateOpportunities(thesis)

	if len(thesis.Decisions) > 0 {
		if err := planner.allocator.Allocate(thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal, "failed to allocate", err,
			))
		}
	}

	planner.arbiter.Arbitrate(thesis)

	/*
		Arbitration is the last hand on the slice, so what survives it is what
		this tick decided. The stamp is what the no_decision branch below reads
		to tell a tick that judged the market and stood down from one that never
		got to judge it at all.
	*/
	thesis.Readiness.Decisions = len(thesis.Decisions) > 0

	/*
		Every decision leaving the planner carries a durable identifier, which
		is what later links a fill, a position, and a post-mortem back to the
		reasoning that produced them. Stamping after arbitration covers each
		path into the slice at the one point they all pass through, including
		the exits arbitration itself raises to rotate an incumbent out.
	*/
	for index := range thesis.Decisions {
		thesis.Decisions[index].EnsureID()
		thesis.Decisions[index].ArbitrationRound = thesis.Tick
	}

	if !thesis.Readiness.Decisions {
		planner.complete(thesis, true, "no_decision")

		// The tick was evaluated and produced nothing, so its evidence has
		// been spent. Carrying it forward would let one tick's readings
		// accumulate into the next and blur every trend drawn from them.
		thesis.Reset()

		return thesis
	}

	if planner.recorder != nil {
		errnie.Error(audit.Record(planner.recorder, "decision", map[string]any{
			"tick":       thesis.Tick,
			"at":         thesis.At,
			"decisions":  thesis.Decisions,
			"categories": thesis.Categories,
		}))
	}

	planner.complete(thesis, true, "decisions")
	planner.publish(thesis)

	// Subscribers receive their own copy, because the reset below empties
	// the slice this thesis holds and would otherwise clear the decisions
	// out from under whoever is still acting on them.
	decisions := slices.Clone(thesis.Decisions)

	planner.subscribers.Range(func(key, value any) bool {
		if subscribers, ok := value.([]*types.Subscription[any]); ok {
			for _, subscriber := range subscribers {
				subscriber.Send(decisions)
			}
		}

		return true
	})

	thesis.Reset()

	return thesis
}

func (planner *Planner) complete(
	thesis *types.Thesis,
	evaluated bool,
	outcome string,
) {
	if planner == nil || thesis == nil {
		return
	}

	errnie.Error(audit.Phase(
		planner.recorder,
		thesis.Tick,
		"decide_end",
		map[string]any{
			"evaluated": evaluated,
			"outcome":   outcome,
			"readiness": thesis.Readiness,
			"decisions": len(thesis.Decisions),
		},
	))

	if planner.ui == nil {
		return
	}

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"tick", thesis.Tick,
		"at", thesis.At,
		"evaluated", evaluated,
		"outcome", outcome,
		"readiness", thesis.Readiness,
		"decisions", len(thesis.Decisions),
	)))
}

func (planner *Planner) publish(thesis *types.Thesis) {
	if planner == nil || planner.ui == nil || thesis == nil || len(thesis.Decisions) == 0 {
		return
	}

	out := datura.NewMap()
	out["decisions"] = thesis.Decisions
	utils.Publish(planner.ui, out)
}
