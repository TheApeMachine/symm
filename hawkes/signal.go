package hawkes

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

const hawkesSource = "hawkes"

type symbolState struct {
	pair      asset.Pair
	state     *HawkesSymbol
	ticks     []trade.Data
	imbalance float64
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
	symbols     map[string]*symbolState
	calibration engine.CalibrationParams
}

var (
	_ engine.System = (*Hawkes)(nil)
	_ engine.Signal = (*Hawkes)(nil)
)

func NewHawkes(ctx context.Context, pool *qpool.Q) *Hawkes {
	ctx, cancel := context.WithCancel(ctx)

	hawkes := &Hawkes{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     make(map[string]*symbolState),
		calibration: engine.DefaultCalibrationParams(),
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		hawkes.subscribers[channel] = group.Subscribe("hawkes:"+channel, 128)
	}

	hawkes.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return hawkes
}

func (hawkes *Hawkes) Start() error {
	return nil
}

func (hawkes *Hawkes) State() engine.State {
	return engine.READY
}

func (hawkes *Hawkes) Tick() error {
	select {
	case <-hawkes.ctx.Done():
		return hawkes.ctx.Err()
	case value := <-hawkes.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair != nil {
				hawkes.symbols[symbol] = &symbolState{
					pair:  *pair,
					state: NewHawkesSymbol(hawkes.calibration),
					ticks: make([]trade.Data, 0, 128),
				}
			}
		}
	case value := <-hawkes.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		symbolState := hawkes.symbols[row.Symbol]

		if symbolState == nil {
			return nil
		}

		symbolState.state.FeedTicker(row.Last, row.Volume)
	case value := <-hawkes.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		symbolState := hawkes.symbols[tick.Symbol]

		if symbolState == nil {
			return nil
		}

		symbolState.ticks = append(symbolState.ticks, tick)

		if len(symbolState.ticks) > 512 {
			symbolState.ticks = symbolState.ticks[len(symbolState.ticks)-512:]
		}
	case value := <-hawkes.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		symbolState := hawkes.symbols[delta.Symbol]

		if symbolState == nil || len(delta.Bids) == 0 || len(delta.Asks) == 0 {
			return nil
		}

		total := delta.Bids[0].Volume + delta.Asks[0].Volume

		if total > 0 {
			symbolState.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
		}
	case value := <-hawkes.subscribers["feedback"].Incoming:
		feedback := value.Value.(engine.PredictionFeedback)

		if feedback.Source != hawkesSource || feedback.Symbol == "" {
			return nil
		}

		symbolState := hawkes.symbols[feedback.Symbol]

		if symbolState == nil {
			return nil
		}

		symbolState.state.ApplyFeedback(feedback)
	default:
		for measurement := range hawkes.Measure() {
			hawkes.broadcasts["measurements"].Send(&qpool.QValue[any]{
				Value: measurement,
			})
		}
	}

	return nil
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

		for _, symbolState := range hawkes.symbols {
			measurement, ok := symbolState.state.Measure(
				symbolState.ticks,
				symbolState.imbalance,
				now,
				symbolState.pair,
			)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (hawkes *Hawkes) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != hawkesSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	symbolState := hawkes.symbols[feedback.Symbol]

	if symbolState == nil {
		return
	}

	symbolState.state.ApplyFeedback(feedback)
}
