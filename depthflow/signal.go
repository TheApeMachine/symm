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
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	pendingMu   sync.Mutex
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

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("depthflow: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
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

					depthflow.queuePending(symbol)
				}

				depthflow.publishPulse()
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

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("depthflow: invalid ticker payload: %T", value.Value))
					continue
				}

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

				delta, deltaOK := value.Value.(market.BookLevelsDelta)
				if !deltaOK {
					errnie.Error(fmt.Errorf("depthflow: invalid book payload: %T", value.Value))
					continue
				}

				raw, ok := depthflow.symbols.Load(delta.Symbol)

				if ok {
					state := raw.(*DepthSymbol)
					state.SetBook(
						selectIfOK(delta.Bids, delta.BidOK),
						selectIfOK(delta.Asks, delta.AskOK),
					)

					if _, seen := depthflow.requested.Load(delta.Symbol); !seen &&
						state.HasBook() {
						depthflow.requested.Store(delta.Symbol, struct{}{})
						depthflow.broadcasts["subscriptions"].Send(
							&qpool.QValue[any]{Value: []string{delta.Symbol}},
						)
						depthflow.publishPulse()
					}
				}

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

				tick, tickOK := value.Value.(trade.Data)
				if !tickOK {
					errnie.Error(fmt.Errorf("depthflow: invalid trade payload: %T", value.Value))
					continue
				}

				raw, ok := depthflow.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*DepthSymbol)
					sign := -1.0

					if tick.Side == "buy" {
						sign = 1.0
					}

					if _, err := state.PushTradePressure(sign); err != nil {
						errnie.Error(err)
					}

					depthflow.publishPulse()
				}

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

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("depthflow: invalid feedback payload: %T", value.Value))
					continue
				}

				depthflow.Feedback(fb)
				depthflow.publishPulse()
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
	symbols := depthflow.pendingBatch(scanCap, requested)

	if len(symbols) > 0 {
		for _, symbol := range symbols {
			depthflow.requested.Store(symbol, struct{}{})
		}

		depthflow.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	depthflow.publishMeasurements()
}

func (depthflow *DepthFlow) queuePending(symbol string) {
	depthflow.pendingMu.Lock()
	defer depthflow.pendingMu.Unlock()

	depthflow.pending = append(depthflow.pending, symbol)
}

func (depthflow *DepthFlow) pendingBatch(scanCap, requested int) []string {
	if requested >= scanCap {
		return nil
	}

	depthflow.pendingMu.Lock()
	defer depthflow.pendingMu.Unlock()

	if len(depthflow.pending) == 0 {
		return nil
	}

	remaining := scanCap - requested
	batch := min(min(config.System.SubscribeBatch, remaining), len(depthflow.pending))
	symbols := append([]string(nil), depthflow.pending[:batch]...)
	depthflow.pending = depthflow.pending[batch:]

	return symbols
}

func (depthflow *DepthFlow) publishMeasurements() {
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

		depthflow.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})

		return true
	})
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

	if err := state.ApplyFeedback(feedback.PredictedReturn, feedback.ActualReturn); err != nil {
		errnie.Error(err)
	}
}
