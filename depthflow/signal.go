package depthflow

import (
	"context"
	"iter"
	"time"

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
	symbols     map[string]*DepthSymbol
	pending     []string
	requested   map[string]struct{}
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
		symbols:     make(map[string]*DepthSymbol),
		requested:   make(map[string]struct{}),
	}

	for _, channel := range []string{"symbols", "book", "trade", "feedback"} {
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
	select {
	case <-depthflow.ctx.Done():
		return depthflow.ctx.Err()
	case value := <-depthflow.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			depthflow.symbols[symbol] = NewDepthSymbol(*pair)

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := depthflow.requested[symbol]; seen {
				continue
			}

			depthflow.pending = append(depthflow.pending, symbol)
		}
	case value := <-depthflow.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		state := depthflow.symbols[delta.Symbol]

		if state == nil {
			break
		}

		if delta.BidOK {
			state.bids = delta.Bids
		}

		if delta.AskOK {
			state.asks = delta.Asks
		}

		if _, seen := depthflow.requested[delta.Symbol]; seen {
			break
		}

		if len(state.bids) == 0 || len(state.asks) == 0 {
			break
		}

		depthflow.requested[delta.Symbol] = struct{}{}
		depthflow.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})
	case value := <-depthflow.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		state := depthflow.symbols[tick.Symbol]

		if state == nil {
			break
		}

		sign := -1.0

		if tick.Side == "buy" {
			sign = 1.0
		}

		state.buyPressure, _ = state.pressure.Next(0, sign)
	case value := <-depthflow.subscribers["feedback"].Incoming:
		depthflow.Feedback(value.Value.(engine.PredictionFeedback))
	default:
	}

	depthflow.publishPulse()

	return nil
}

func (depthflow *DepthFlow) publishPulse() {
	scanCap := config.System.MaxScanSymbols / 8

	if scanCap < 1 {
		scanCap = 1
	}

	if len(depthflow.pending) > 0 && len(depthflow.requested) < scanCap {
		remaining := scanCap - len(depthflow.requested)
		batch := config.System.SubscribeBatch

		if batch > remaining {
			batch = remaining
		}

		if batch > len(depthflow.pending) {
			batch = len(depthflow.pending)
		}

		symbols := depthflow.pending[:batch]
		depthflow.pending = depthflow.pending[batch:]

		for _, symbol := range symbols {
			depthflow.requested[symbol] = struct{}{}
		}

		depthflow.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	for measurement := range depthflow.Measure() {
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
		for symbol, state := range depthflow.symbols {
			if _, subscribed := depthflow.requested[symbol]; !subscribed {
				continue
			}

			measurement, ok := state.Measure()

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (depthflow *DepthFlow) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != depthflowSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	state := depthflow.symbols[feedback.Symbol]

	if state == nil {
		return
	}

	_, _ = state.forecast.Next(0, feedback.PredictedReturn, feedback.ActualReturn)
}
