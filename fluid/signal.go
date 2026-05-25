package fluid

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

const fluidSource = "fluid"

/*
Fluid scores multi-level book flow pressure and spread tightness.
*/
type Fluid struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*FluidSymbol
}

var (
	_ engine.System = (*Fluid)(nil)
	_ engine.Signal = (*Fluid)(nil)
)

func NewFluid(ctx context.Context, pool *qpool.Q) *Fluid {
	ctx, cancel := context.WithCancel(ctx)

	fluid := &Fluid{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*FluidSymbol),
	}

	for _, channel := range []string{"symbols", "book", "trade", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		fluid.subscribers[channel] = group.Subscribe("fluid:"+channel, 128)
	}

	fluid.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return fluid
}

func (fluid *Fluid) Start() error  { return nil }
func (fluid *Fluid) State() engine.State { return engine.READY }

func (fluid *Fluid) Tick() error {
	select {
	case <-fluid.ctx.Done():
		return fluid.ctx.Err()
	case value := <-fluid.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair != nil {
				fluid.symbols[symbol] = NewFluidSymbol(*pair)
			}
		}
	case value := <-fluid.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		state := fluid.symbols[delta.Symbol]

		if state == nil {
			return nil
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
	case value := <-fluid.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		state := fluid.symbols[tick.Symbol]

		if state == nil {
			return nil
		}

		sign := -1.0

		if tick.Side == "buy" {
			sign = 1.0
		}

		state.buyPressure, _ = state.pressure.Next(0, sign)
	case value := <-fluid.subscribers["feedback"].Incoming:
		fluid.Feedback(value.Value.(engine.PredictionFeedback))
	default:
		for measurement := range fluid.Measure() {
			fluid.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}
	}

	return nil
}

func (fluid *Fluid) Close() error {
	fluid.cancel()
	return nil
}

func (fluid *Fluid) Source() string { return fluidSource }

func (fluid *Fluid) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		for _, state := range fluid.symbols {
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
