package fluid

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
	symbols     sync.Map
	pending     []string
	requested   sync.Map
}

func NewFluid(ctx context.Context, pool *qpool.Q) *Fluid {
	ctx, cancel := context.WithCancel(ctx)

	fluid := &Fluid{
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

			fluid.symbols.Store(symbol, NewFluidSymbol(*pair))

			if pair.Quote != config.System.QuoteCurrency {
				continue
			}

			if _, seen := fluid.requested.Load(symbol); seen {
				continue
			}

			fluid.pending = append(fluid.pending, symbol)
		}

		fluid.publishPulse()
	case value := <-fluid.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		raw, ok := fluid.symbols.Load(row.Symbol)

		if !ok {
			break
		}

		state := raw.(*FluidSymbol)
		state.changePct = row.ChangePct
		state.volume = row.Volume

		fluid.publishPulse()
	case value := <-fluid.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		raw, ok := fluid.symbols.Load(delta.Symbol)

		if !ok {
			break
		}

		state := raw.(*FluidSymbol)

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

		if _, seen := fluid.requested.Load(delta.Symbol); seen {
			break
		}

		if len(state.bids) == 0 || len(state.asks) == 0 {
			break
		}

		fluid.requested.Store(delta.Symbol, struct{}{})
		fluid.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})

		fluid.publishPulse()
	case value := <-fluid.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		raw, ok := fluid.symbols.Load(tick.Symbol)

		if !ok {
			break
		}

		state := raw.(*FluidSymbol)
		sign := -1.0

		if tick.Side == "buy" {
			sign = 1.0
		}

		state.buyPressure = errnie.Does(func() (float64, error) {
			return state.pressure.Next(0, sign)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		fluid.publishPulse()
	case value := <-fluid.subscribers["feedback"].Incoming:
		fluid.Feedback(value.Value.(engine.PredictionFeedback))
	default:
		errnie.Warn("this just feels like, spinning plates, system=fluid")
	}

	return nil
}

func (fluid *Fluid) requestedCount() int {
	count := 0

	fluid.requested.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

func (fluid *Fluid) publishPulse() {
	if len(fluid.pending) > 0 && fluid.requestedCount() < config.System.MaxScanSymbols {
		remaining := config.System.MaxScanSymbols - fluid.requestedCount()
		batch := min(min(config.System.SubscribeBatch, remaining), len(fluid.pending))

		symbols := fluid.pending[:batch]
		fluid.pending = fluid.pending[batch:]

		for _, symbol := range symbols {
			fluid.requested.Store(symbol, struct{}{})
		}

		fluid.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: symbols})
	}

	fluid.publishMeasurements()
	fluid.publishFieldRows()
}

func (fluid *Fluid) publishMeasurements() {
	waiters := make([]chan *qpool.QValue[any], 0)

	fluid.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := fluid.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*FluidSymbol)
		waiters = append(
			waiters,
			fluid.pool.ScheduleFast(fluid.ctx, func(ctx context.Context) (any, error) {
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

		fluid.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

func (fluid *Fluid) publishFieldRows() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	symbols := make([]map[string]any, 0)
	waiters := make([]struct {
		symbol string
		waiter chan *qpool.QValue[any]
	}, 0)

	fluid.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := fluid.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*FluidSymbol)
		waiters = append(waiters, struct {
			symbol string
			waiter chan *qpool.QValue[any]
		}{
			symbol: symbol,
			waiter: fluid.pool.ScheduleFast(fluid.ctx, func(ctx context.Context) (any, error) {
				return state.wireRow(), nil
			}),
		})

		return true
	})

	for _, job := range waiters {
		value := <-job.waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		row, ok := value.Value.(map[string]any)

		if !ok || row == nil {
			continue
		}

		fluid.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "field_row",
			"ts":     now,
			"symbol": job.symbol,
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
		fluid.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := fluid.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*FluidSymbol)
			measurement, ok := state.Measure()

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

func (fluid *Fluid) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != fluidSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := fluid.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*FluidSymbol)
	state.ApplyFeedback(feedback)
}
