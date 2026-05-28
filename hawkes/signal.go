package hawkes

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

const hawkesSource = "hawkes"

type symbolState struct {
	pair      asset.Pair
	state     *HawkesSymbol
	ticks     []trade.Data
	imbalance float64
	last      float64
	bid       float64
	ask       float64
}

/*
Hawkes detects buy-side trade clustering via a bivariate self-exciting Hawkes model.
*/
type Hawkes struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	calibration engine.CalibrationParams
	requested   sync.Map
}

func NewHawkes(ctx context.Context, pool *qpool.Q) *Hawkes {
	ctx, cancel := context.WithCancel(ctx)

	hawkes := &Hawkes{
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
		hawkes.subscribers[channel] = group.Subscribe("hawkes:"+channel, 128)
	}

	hawkes.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	hawkes.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return hawkes
}

func (hawkes *Hawkes) Start() error {
	return nil
}

func (hawkes *Hawkes) State() engine.State {
	return engine.READY
}

func (hawkes *Hawkes) Tick() error {
	errnie.Info("starting hawkes tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case value, ok := <-hawkes.subscribers["symbols"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("hawkes symbols channel closed"))
					return
				}

				pairs, pairsOK := value.Value.(map[string]*asset.Pair)
				if !pairsOK {
					errnie.Error(fmt.Errorf("hawkes: invalid symbols payload: %T", value.Value))
					continue
				}

				for symbol, pair := range pairs {
					if pair != nil {
						hawkes.symbols.Store(symbol, &symbolState{
							pair:  *pair,
							state: NewHawkesSymbol(hawkes.calibration),
							ticks: make([]trade.Data, 0, 128),
						})
					}
				}

				hawkes.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case value, ok := <-hawkes.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("hawkes tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("hawkes: invalid ticker payload: %T", value.Value))
					continue
				}

				raw, ok := hawkes.symbols.Load(row.Symbol)

				if ok {
					symbolState := raw.(*symbolState)
					symbolState.state.FeedTicker(row.Last, row.Volume)

					if row.Last > 0 {
						symbolState.last = row.Last
					}

					if row.Bid > 0 {
						symbolState.bid = row.Bid
					}

					if row.Ask > 0 {
						symbolState.ask = row.Ask
					}

					if _, seen := hawkes.requested.Load(row.Symbol); !seen && row.Volume > 0 {
						if pair := symbolState.pair; pair.Quote == config.System.QuoteCurrency {
							hawkes.requested.Store(row.Symbol, struct{}{})
							hawkes.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})
							hawkes.publishPulse()
						}
					}
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case value, ok := <-hawkes.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("hawkes trade channel closed"))
					return
				}

				tick, tickOK := value.Value.(trade.Data)
				if !tickOK {
					errnie.Error(fmt.Errorf("hawkes: invalid trade payload: %T", value.Value))
					continue
				}

				raw, ok := hawkes.symbols.Load(tick.Symbol)

				if ok {
					symbolState := raw.(*symbolState)
					symbolState.ticks = append(symbolState.ticks, tick)

					if len(symbolState.ticks) > 512 {
						symbolState.ticks = symbolState.ticks[len(symbolState.ticks)-512:]
					}

					if _, seen := hawkes.requested.Load(tick.Symbol); !seen && len(symbolState.ticks) >= 16 {
						hawkes.requested.Store(tick.Symbol, struct{}{})
						hawkes.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
					}

					hawkes.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case value, ok := <-hawkes.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("hawkes book channel closed"))
					return
				}

				delta, deltaOK := value.Value.(market.BookLevelsDelta)
				if !deltaOK {
					errnie.Error(fmt.Errorf("hawkes: invalid book payload: %T", value.Value))
					continue
				}

				raw, ok := hawkes.symbols.Load(delta.Symbol)

				if ok && len(delta.Bids) > 0 && len(delta.Asks) > 0 {
					symbolState := raw.(*symbolState)
					symbolState.bid = delta.Bids[0].Price
					symbolState.ask = delta.Asks[0].Price

					if symbolState.last <= 0 {
						symbolState.last = (symbolState.bid + symbolState.ask) / 2
					}

					total := delta.Bids[0].Volume + delta.Asks[0].Volume

					if total > 0 {
						symbolState.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
					}

					hawkes.publishPulse()
				}

			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case value, ok := <-hawkes.subscribers["feedback"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("hawkes feedback channel closed"))
					return
				}

				fb, fbOK := value.Value.(engine.PredictionFeedback)
				if !fbOK {
					errnie.Error(fmt.Errorf("hawkes: invalid feedback payload: %T", value.Value))
					continue
				}

				hawkes.Feedback(fb)
				hawkes.publishPulse()
			}
		}
	})

	wg.Wait()
	return hawkes.ctx.Err()
}

func (hawkes *Hawkes) publishPulse() {
	hawkes.publishMeasurements()
}

func (hawkes *Hawkes) publishMeasurements() {
	now := time.Now()

	hawkes.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := hawkes.requested.Load(symbol); !subscribed {
			return true
		}

		symbolState := value.(*symbolState)
		measurement, ok := symbolState.state.Measure(
			symbolState.ticks,
			symbolState.imbalance,
			now,
			symbolState.pair,
		)

		if !ok {
			return true
		}

		measurement.Last = symbolState.last
		measurement.Bid = symbolState.bid
		measurement.Ask = symbolState.ask

		if measurement.Last <= 0 && measurement.Bid > 0 && measurement.Ask > 0 {
			measurement.Last = (measurement.Bid + measurement.Ask) / 2
		}

		hawkes.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})

		return true
	})
}

func (hawkes *Hawkes) Close() error {
	hawkes.cancel()
	return nil
}

func (hawkes *Hawkes) Source() string {
	return hawkesSource
}

func (hawkes *Hawkes) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		now := time.Now()

		hawkes.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := hawkes.requested.Load(symbol); !subscribed {
				return true
			}

			symbolState := value.(*symbolState)
			measurement, ok := symbolState.state.Measure(
				symbolState.ticks,
				symbolState.imbalance,
				now,
				symbolState.pair,
			)

			if !ok {
				return true
			}

			return yield(measurement)
		})
	}
}

func (hawkes *Hawkes) Feedback(feedback engine.PredictionFeedback) {
	if !engine.FeedbackIncludesSource(feedback, hawkesSource) || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := hawkes.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	symbolState := raw.(*symbolState)
	symbolState.state.ApplyFeedback(feedback)
}
