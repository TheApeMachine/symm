package causal

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
)

const causalSource = "causal"

/*
Causal scores Pearl's ladder: association, intervention, counterfactual uplift.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as backdoor control.
*/
type Causal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	calibration engine.CalibrationParams
	pending     []string
	requested   sync.Map
}

func NewCausal(ctx context.Context, pool *qpool.Q) *Causal {
	ctx, cancel := context.WithCancel(ctx)

	causal := &Causal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		calibration: engine.DefaultCalibrationParams(),
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		causal.subscribers[channel] = group.Subscribe("causal:"+channel, 128)
	}

	for _, channel := range []string{"measurements", "subscriptions"} {
		causal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	return causal
}

func (causal *Causal) Start() error        { return nil }
func (causal *Causal) State() engine.State { return engine.READY }

func (causal *Causal) Tick() error {
	errnie.Info("starting causal tick")

	for {
		select {
		case <-causal.ctx.Done():
			return causal.ctx.Err()
		case value := <-causal.subscribers["symbols"].Incoming:
			for symbol, pair := range value.Value.(map[string]*asset.Pair) {
				if pair == nil {
					continue
				}

				causal.symbols.Store(symbol, NewCausalSymbol(*pair, causal.calibration))

				if pair.Quote != config.System.QuoteCurrency {
					continue
				}

				if _, seen := causal.requested.Load(symbol); seen {
					continue
				}

				causal.pending = append(causal.pending, symbol)
			}

			causal.publishPulse()
		case value := <-causal.subscribers["tick"].Incoming:
			row := value.Value.(market.TickerRow)
			raw, ok := causal.symbols.Load(row.Symbol)

			if !ok {
				break
			}

			state := raw.(*CausalSymbol)
			state.FeedTicker(row)

			if _, seen := causal.requested.Load(row.Symbol); seen || state.changePct == 0 {
				break
			}

			causal.requested.Store(row.Symbol, struct{}{})
			causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})

			causal.publishPulse()
		case value := <-causal.subscribers["trade"].Incoming:
			tick := value.Value.(trade.Data)
			raw, ok := causal.symbols.Load(tick.Symbol)

			if !ok {
				break
			}

			state := raw.(*CausalSymbol)
			state.FeedTrade(tick)

			if _, seen := causal.requested.Load(tick.Symbol); seen {
				break
			}

			causal.requested.Store(tick.Symbol, struct{}{})
			causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})

			causal.publishPulse()
		case value := <-causal.subscribers["book"].Incoming:
			delta := value.Value.(market.BookLevelsDelta)
			raw, ok := causal.symbols.Load(delta.Symbol)

			if !ok {
				break
			}

			state := raw.(*CausalSymbol)
			state.FeedBook(delta)

			if _, seen := causal.requested.Load(delta.Symbol); seen {
				break
			}

			if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
				break
			}

			causal.requested.Store(delta.Symbol, struct{}{})
			causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})

			causal.publishPulse()
		case value := <-causal.subscribers["feedback"].Incoming:
			causal.Feedback(value.Value.(engine.PredictionFeedback))
			causal.publishPulse()
		}
	}
}

func (causal *Causal) requestedCount() int {
	count := 0

	causal.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (causal *Causal) publishPulse() {
	scanCap := max(config.System.MaxScanSymbols/8, 1)
	requested := causal.requestedCount()

	if len(causal.pending) > 0 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(causal.pending))

		symbols := causal.pending[:batch]
		causal.pending = causal.pending[batch:]

		for _, symbol := range symbols {
			causal.requested.Store(symbol, struct{}{})
		}

		causal.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	causal.publishMeasurements()
}

func (causal *Causal) publishMeasurements() {
	macro := causal.macroMomentum()
	now := time.Now()
	waiters := make([]chan *qpool.QValue[any], 0)

	causal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := causal.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*CausalSymbol)
		waiters = append(
			waiters,
			causal.pool.ScheduleFast(causal.ctx, func(ctx context.Context) (any, error) {
				measurement, ok := state.Measure(macro, now)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)

		return true
	})

	for _, waiter := range waiters {
		value := <-waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		measurement, ok := value.Value.(engine.Measurement)

		if !ok {
			continue
		}

		causal.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (causal *Causal) Close() error {
	causal.cancel()
	return nil
}

func (causal *Causal) Source() string { return causalSource }

func (causal *Causal) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		macro := causal.macroMomentum()
		now := time.Now()

		causal.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := causal.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*CausalSymbol)
			measurement, ok := state.Measure(macro, now)

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

func (causal *Causal) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != causalSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := causal.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*CausalSymbol)
	state.ApplyFeedback(feedback)
}

func (causal *Causal) macroMomentum() float64 {
	changes := make([]float64, 0)

	causal.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)

		if state.changePct != 0 {
			changes = append(changes, state.changePct)
		}

		return true
	})

	if len(changes) == 0 {
		return 0
	}

	return numeric.PercentileSorted(numeric.CopySorted(changes), 0.5)
}
