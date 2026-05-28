package depthflow

import (
	"context"
	"fmt"
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
)

const depthflowSource = "depthflow"

/*
DepthFlow detects multi-level order-book imbalance and depth-weighted flow pressure.
*/
type DepthFlow struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	pending     []string
	requested   sync.Map
}

/*
NewDepthFlow wires broadcast subscribers for the depth-flow system.
*/
func NewDepthFlow(ctx context.Context, pool *qpool.Q) *DepthFlow {
	ctx, cancel := context.WithCancel(ctx)

	depthflow := &DepthFlow{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "book", "trade", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		depthflow.subscribers[channel] = group.Subscribe("depthflow:"+channel, 128)
	}

	depthflow.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	depthflow.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return depthflow
}

func (depthflow *DepthFlow) Start() error {
	return nil
}

func (depthflow *DepthFlow) State() engine.State {
	return engine.READY
}

func (depthflow *DepthFlow) Tick() error {
	errnie.Info("starting depthflow tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-depthflow.ctx.Done():
				return
			case value, ok := <-depthflow.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("depthflow symbols channel closed"))
					return
				}

				depthflow.mu.Lock()
				for symbol, pair := range value.Value.(map[string]*asset.Pair) {
					if pair == nil {
						continue
					}

					depthflow.symbols.Store(symbol, NewDepthSymbol(*pair))

					if pair.Quote != config.System.QuoteCurrency {
						continue
					}

					if _, seen := depthflow.requested.Load(symbol); seen {
						continue
					}

					depthflow.pending = append(depthflow.pending, symbol)
				}

				depthflow.publishPulse()
				depthflow.mu.Unlock()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-depthflow.ctx.Done():
				return
			case value, ok := <-depthflow.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("depthflow tick channel closed"))
					return
				}

				depthflow.mu.Lock()
				row := value.Value.(market.TickerRow)
				raw, ok := depthflow.symbols.Load(row.Symbol)

				if ok {
					state := raw.(*DepthSymbol)
					state.FeedTicker(row)

					if _, seen := depthflow.requested.Load(row.Symbol); !seen && row.ChangePct != 0 {
						depthflow.requested.Store(row.Symbol, struct{}{})
						depthflow.broadcasts["subscriptions"].Send(
							&qpool.QValue[any]{Value: []string{row.Symbol}},
						)
						depthflow.publishPulse()
					}
				}

				depthflow.mu.Unlock()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-depthflow.ctx.Done():
				return
			case value, ok := <-depthflow.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("depthflow book channel closed"))
					return
				}

				depthflow.mu.Lock()
				delta := value.Value.(market.BookLevelsDelta)
				raw, ok := depthflow.symbols.Load(delta.Symbol)

				if ok {
					state := raw.(*DepthSymbol)

					if delta.BidOK {
						state.bids = delta.Bids
					}

					if delta.AskOK {
						state.asks = delta.Asks
					}

					if _, seen := depthflow.requested.Load(delta.Symbol); !seen &&
						len(state.bids) > 0 && len(state.asks) > 0 {
						depthflow.requested.Store(delta.Symbol, struct{}{})
						depthflow.broadcasts["subscriptions"].Send(
							&qpool.QValue[any]{Value: []string{delta.Symbol}},
						)
						depthflow.publishPulse()
					}
				}

				depthflow.mu.Unlock()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-depthflow.ctx.Done():
				return
			case value, ok := <-depthflow.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("depthflow trade channel closed"))
					return
				}

				depthflow.mu.Lock()
				tick := value.Value.(trade.Data)
				raw, ok := depthflow.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*DepthSymbol)
					sign := -1.0

					if tick.Side == "buy" {
						sign = 1.0
					}

					state.buyPressure = errnie.Does(func() (float64, error) {
						return state.pressure.Next(0, sign)
					}).Or(func(err error) {
						errnie.Error(err)
					}).Value()

					depthflow.publishPulse()
				}

				depthflow.mu.Unlock()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-depthflow.ctx.Done():
				return
			case value, ok := <-depthflow.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("depthflow feedback channel closed"))
					return
				}

				depthflow.mu.Lock()
				depthflow.Feedback(value.Value.(engine.PredictionFeedback))
				depthflow.publishPulse()
				depthflow.mu.Unlock()
			}
		}
	})

	wg.Wait()
	return depthflow.ctx.Err()
}

func (depthflow *DepthFlow) requestedCount() int {
	count := 0

	depthflow.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (depthflow *DepthFlow) publishPulse() {
	scanCap := max(config.System.MaxScanSymbols/8, 1)
	requested := depthflow.requestedCount()

	if len(depthflow.pending) > 0 && requested < scanCap {
		remaining := scanCap - requested
		batch := min(min(config.System.SubscribeBatch, remaining), len(depthflow.pending))

		symbols := depthflow.pending[:batch]
		depthflow.pending = depthflow.pending[batch:]

		for _, symbol := range symbols {
			depthflow.requested.Store(symbol, struct{}{})
		}

		depthflow.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	depthflow.publishMeasurements()
}

func (depthflow *DepthFlow) publishMeasurements() {
	waiters := make([]chan *qpool.QValue[any], 0)

	depthflow.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := depthflow.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*DepthSymbol)
		waiters = append(
			waiters,
			depthflow.pool.ScheduleFast(depthflow.ctx, func(ctx context.Context) (any, error) {
				measurement, ok := state.Measure()

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

		depthflow.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (depthflow *DepthFlow) Close() error {
	depthflow.cancel()
	return nil
}

func (depthflow *DepthFlow) Source() string {
	return depthflowSource
}

func (depthflow *DepthFlow) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		depthflow.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := depthflow.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*DepthSymbol)
			measurement, ok := state.Measure()

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

func (depthflow *DepthFlow) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, depthflowSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := depthflow.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*DepthSymbol)

	if _, err := state.forecast.Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}
