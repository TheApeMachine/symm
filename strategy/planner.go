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
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	api           *websocket.API
	desk          *broker.Desk
	price         *broker.Price
	balance       *broker.Balance
	recorder      *audit.Recorder
	evaluator     Evaluator
	arbiter       *Arbiter
	allocator     *Allocator
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
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		ui:     uiHub,
		subscriptions: map[string]*types.Subscription[any]{
			"analyzer": analyzer.Subscribe(
				"analyzer", types.NewLatestSubscription[any](),
			),
		},
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

	planner.run()
	return planner
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

func (planner *Planner) run() {
	go func() {
		for {
			select {
			case <-planner.ctx.Done():
				return
			case in := <-planner.subscriptions["analyzer"].Channel:
				thesis, ok := in.(*types.Thesis)

				if !ok {
					continue
				}

				planner.Update(thesis)
			}
		}
	}()
}

func (planner *Planner) Close() error {
	planner.cancel()
	return nil
}

func (planner *Planner) Update(thesis *types.Thesis) {
	if !thesis.LogicAnalyzed() {
		return
	}

	planner.evaluator.ManageContinuation(thesis, planner.desk, planner.price)
	planner.evaluator.EvaluateOpportunities(thesis)

	if err := planner.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "failed to allocate", err,
		))
	}

	planner.arbiter.Arbitrate(thesis)

	// Arbitration is the last hand on the verdicts, so what it leaves is what
	// this tick decided. Detaching them here gives subscribers their own copy,
	// because the reset below clears the map the moment the cycle completes.
	decisions := planner.decisions(thesis)
	thesis.Stamp(types.SourcePlanner)
	planner.subscribers.Range(func(key, value any) bool {
		name, ok := key.(string)

		if !ok || name == "planner" {
			return true
		}

		for _, subscriber := range value.([]*types.Subscription[any]) {
			subscriber.SendLatest(decisions)
		}

		return true
	})

	terminalDecision := slices.ContainsFunc(
		decisions,
		func(decision types.Decision) bool {
			return decision.Action != types.ActionNothing
		},
	)

	if len(decisions) == 0 || !terminalDecision {
		return
	}

	thesis.Stamp(types.SourcePlanner)
	utils.Fanout(planner.subscribers, "planner", thesis)

	utils.Publish(planner.ui, datura.NewMap("strategy", datura.NewMap(
		"evaluated", true,
		"outcome", "decisions",
		"decisions", decisions,
	)))
}

/*
decisions detaches this tick's verdicts and gives each the identity that later
links a fill, a position and a post-mortem back to the reasoning that produced
it. Every path into the map passes through here, so it is the one place the
identity has to be applied.
*/
func (planner *Planner) decisions(thesis *types.Thesis) []types.Decision {
	decisions := make([]types.Decision, 0)

	thesis.Decisions.Range(func(key, value any) bool {
		decision, ok := value.(*types.Decision)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"planner: decision map holds a value that is not a decision",
				nil,
			))

			return true
		}

		decision.EnsureID()
		decision.ArbitrationRound = thesis.Tick
		decisions = append(decisions, *decision)

		return true
	})

	return decisions
}
