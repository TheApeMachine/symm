package fluid

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
	errnie.Info("starting fluid tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-fluid.ctx.Done():
				return
			case value, ok := <-fluid.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("fluid symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("fluid: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
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
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-fluid.ctx.Done():
				return
			case value, ok := <-fluid.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("fluid tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("fluid: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := fluid.symbols.Load(row.Symbol)

				if ok {
					state := raw.(*FluidSymbol)
					state.FeedTicker(row)

					fluid.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-fluid.ctx.Done():
				return
			case value, ok := <-fluid.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("fluid book channel closed"))
					return
				}

				delta, deltaOK := value.Value.(market.BookLevelsDelta)
				if !deltaOK {
					errnie.Error(fmt.Errorf("fluid: invalid book payload: %T", value.Value))
					continue
				}

				raw, ok := fluid.symbols.Load(delta.Symbol)

				if ok {
					state := raw.(*FluidSymbol)
					state.FeedBook(delta)

					if _, seen := fluid.requested.Load(delta.Symbol); !seen && state.HasBook() {
						fluid.requested.Store(delta.Symbol, struct{}{})
						fluid.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{delta.Symbol}})
						fluid.publishPulse()
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-fluid.ctx.Done():
				return
			case value, ok := <-fluid.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("fluid trade channel closed"))
					return
				}

				tick, tickOK := value.Value.(trade.Data)
				if !tickOK {
					errnie.Error(fmt.Errorf("fluid: invalid trade payload: %T", value.Value))
					continue
				}

				raw, ok := fluid.symbols.Load(tick.Symbol)

				if ok {
					state := raw.(*FluidSymbol)
					state.FeedTradeSide(tick.Timestamp, tick.Qty, tick.Side)

					fluid.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-fluid.ctx.Done():
				return
			case value, ok := <-fluid.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("fluid feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("fluid: invalid feedback payload: %T", value.Value))
					continue
				}

				fluid.Feedback(fb)
				fluid.publishPulse()
			}
		}
	})

	wg.Wait()
	return fluid.ctx.Err()
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

		fluid.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})

		return true
	})
}

func (fluid *Fluid) publishFieldRows() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	symbols := make([]map[string]any, 0)

	fluid.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := fluid.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*FluidSymbol)
		row := state.wireRow()

		if row == nil {
			return true
		}

		fluid.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "field_row",
			"ts":     now,
			"symbol": symbol,
			"row":    row,
		}})
		symbols = append(symbols, row)

		return true
	})

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
	if !engine.FeedbackIncludesSource(feedback, fluidSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := fluid.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*FluidSymbol)
	state.ApplyFeedback(feedback)
}
