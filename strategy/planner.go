package strategy

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

type Planner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	subscribers *sync.Map
	subscriptions map[string]*types.Subscription[any]
	runOnce     sync.Once
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
	uiHub chan<- []byte,
	api *websocket.API,
	desk *broker.Desk,
	instrument *broker.Instrument,
	price *broker.Price,
	balance *broker.Balance,
	analyzer *logic.Analyzer,
	recorder *audit.Recorder,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)
	_ = uiHub // Retained for signature compatibility
	buffer := viper.GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	evaluator := NewEvaluator(desk, price, balance)
	arbiter := NewArbiter(desk, price)

	planner := &Planner{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.READY,
		subscribers: &sync.Map{},
		subscriptions: map[string]*types.Subscription[any]{},
		api:         api,
		desk:        desk,
		price:       price,
		balance:     balance,
		recorder:    recorder,
		evaluator:   evaluator,
		arbiter:     arbiter,
		allocator:   NewAllocator(ctx, balance, instrument, price),
	}

	if analyzer != nil {
		planner.AttachAnalyzer(analyzer)
	}

	planner.run()
	return planner
}

func (planner *Planner) AttachAnalyzer(analyzer *logic.Analyzer) {
	if analyzer == nil {
		return
	}

	if planner.subscriptions["thesis"] != nil {
		planner.run()
		return
	}

	planner.subscriptions["thesis"] = analyzer.Subscribe(
		"thesis", types.NewSubscription[any](),
	)

	planner.run()
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

func (planner *Planner) run() {
	if planner.subscriptions["thesis"] == nil {
		return
	}

	planner.runOnce.Do(func() {
		go func() {
			for {
				select {
				case <-planner.ctx.Done():
					return
				case thesis := <-planner.subscriptions["thesis"].Channel:
					if thesis, ok := thesis.(*types.Thesis); ok {
						planner.Update(thesis)
					}
				}
			}
		}()
	})
}

func (planner *Planner) Update(thesis *types.Thesis) *types.Thesis {
	if thesis == nil {
		return nil
	}

	errnie.Error(audit.Phase(
		planner.recorder, thesis.Tick, "decide_begin", nil,
	))

	if thesis.Status != types.READY {
		return thesis
	}

	readiness := thesis.Readiness()

	if !readiness.Manifold || !readiness.Resonance || !readiness.Causal || !readiness.Graph {
		return thesis
	}

	planner.evaluator.ManageContinuation(thesis, planner.desk, planner.price)
	planner.evaluator.EvaluateOpportunities(thesis)
	planner.arbiter.Arbitrate(thesis)

	if err := planner.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "failed to allocate", err,
		))
	}

	if !thesis.Readiness().Decisions {
		return thesis
	}

	if planner.recorder != nil {
		errnie.Error(audit.Record(planner.recorder, "decision", map[string]any{
			"tick":       thesis.Tick,
			"at":         thesis.At,
			"decisions":  thesis.Decisions,
			"forecasts":  thesis.Forecasts,
			"categories": thesis.Categories,
		}))
	}

	planner.subscribers.Range(func(key, value any) bool {
		if subscribers, ok := value.([]*types.Subscription[any]); ok {
			for _, subscriber := range subscribers {
				subscriber.Send(thesis.Decisions)
			}
		}

		return true
	})

	thesis.Reset()

	return thesis
}
