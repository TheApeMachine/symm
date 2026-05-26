package fluid

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

const fluidSource = "fluid"

/*
Fluid applies book-flow dynamics per symbol and streams field_row updates to ui.
*/
type Fluid struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*FluidSymbol
	pending     []string
	requested   map[string]struct{}
}

func NewFluid(ctx context.Context, pool *qpool.Q) *Fluid {
	ctx, cancel := context.WithCancel(ctx)

	fluid := &Fluid{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*FluidSymbol),
		requested:   make(map[string]struct{}),
	}

	for _, channel := range []string{"symbols", "tick", "book", "trade", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		fluid.subscribers[channel] = group.Subscribe("fluid:"+channel, 128)
	}

	fluid.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	fluid.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	fluid.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return fluid
}

func (fluid *Fluid) Start() error        { return nil }
func (fluid *Fluid) State() engine.State { return engine.READY }

func (fluid *Fluid) Tick() error {
	select {
	case <-fluid.ctx.Done():
		return fluid.ctx.Err()
	case value := <-fluid.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair == nil {
				continue
			}

			fluid.symbols[symbol] = NewFluidSymbol(*pair)

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := fluid.requested[symbol]; seen {
				continue
			}

			fluid.pending = append(fluid.pending, symbol)
		}
	case value := <-fluid.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := fluid.symbols[row.Symbol]

		if state == nil {
			break
		}

		state.changePct = row.ChangePct
		state.volume = row.Volume
	case value := <-fluid.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		state := fluid.symbols[delta.Symbol]

		if state == nil {
			break
		}

		if delta.BidOK {
			state.bids = delta.Bids
		}

		if delta.AskOK {
			state.asks = delta.Asks
		}

		if len(state.bids) > 0 && len(state.asks) > 0 {
			bid := state.bids[0].Price
			ask := state.asks[0].Price
			mid := (bid + ask) / 2

			if mid > 0 {
				state.spreadBPS = (ask - bid) / mid * 10000
			}
		}

		if _, seen := fluid.requested[delta.Symbol]; seen {
			break
		}

		if len(state.bids) == 0 || len(state.asks) == 0 {
			break
		}

		fluid.requested[delta.Symbol] = struct{}{}
		fluid.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})
	case value := <-fluid.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		state := fluid.symbols[tick.Symbol]

		if state == nil {
			break
		}

		sign := -1.0

		if tick.Side == "buy" {
			sign = 1.0
		}

		state.buyPressure, _ = state.pressure.Next(0, sign)
	case value := <-fluid.subscribers["feedback"].Incoming:
		fluid.Feedback(value.Value.(engine.PredictionFeedback))
	default:
	}

	fluid.publishPulse()

	return nil
}

func (fluid *Fluid) publishPulse() {
	if len(fluid.pending) > 0 && len(fluid.requested) < config.System.MaxScanSymbols {
		remaining := config.System.MaxScanSymbols - len(fluid.requested)
		batch := config.System.SubscribeBatch

		if batch > remaining {
			batch = remaining
		}

		if batch > len(fluid.pending) {
			batch = len(fluid.pending)
		}

		symbols := fluid.pending[:batch]
		fluid.pending = fluid.pending[batch:]

		for _, symbol := range symbols {
			fluid.requested[symbol] = struct{}{}
		}

		fluid.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	for measurement := range fluid.Measure() {
		fluid.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	fluid.publishFieldRows()
}

func (fluid *Fluid) publishFieldRows() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	symbols := make([]map[string]any, 0)

	for symbol, state := range fluid.symbols {
		if _, subscribed := fluid.requested[symbol]; !subscribed {
			continue
		}

		row := state.wireRow()

		if row == nil {
			continue
		}

		fluid.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "field_row",
			"ts":     now,
			"symbol": symbol,
			"row":    row,
		}})
		symbols = append(symbols, row)
	}

	if len(symbols) == 0 {
		return
	}

	fluid.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":        "field_snapshot",
		"ts":           now,
		"symbol_count": len(symbols),
		"symbols":      symbols,
	}})
}

func (fluid *Fluid) Close() error {
	fluid.cancel()
	return nil
}

func (fluid *Fluid) Source() string { return fluidSource }

func (fluid *Fluid) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		for symbol, state := range fluid.symbols {
			if _, subscribed := fluid.requested[symbol]; !subscribed {
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

func (fluid *Fluid) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != fluidSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	state := fluid.symbols[feedback.Symbol]

	if state == nil {
		return
	}

	state.ApplyFeedback(feedback)
}
