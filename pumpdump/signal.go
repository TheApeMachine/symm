package pumpdump

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const pumpdumpSource = "pumpdump"

const tradeWindow = 5 * time.Minute

var (
	moveClassifier *adaptive.Classifier
	peakGate       = adaptive.NewPeak()
)

func init() {
	var err error

	moveClassifier, err = adaptive.NewClassifier(
		[]float64{-0.001, 0.001},
		[]float64{0, 1, 2},
		[]string{"dump", "precursor", "actual_pump"},
	)

	if err != nil {
		errnie.Error(err)
	}
}

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	requested   sync.Map
}

/*
NewPumpDump wires broadcast subscribers for the pumpdump system.
*/
func NewPumpDump(ctx context.Context, pool *qpool.Q) *PumpDump {
	ctx, cancel := context.WithCancel(ctx)

	pumpdump := &PumpDump{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		symbols:     sync.Map{},
		requested:   sync.Map{},
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book", "feedback"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		pumpdump.subscribers[channel] = group.Subscribe("pumpdump:"+channel, 128)
	}

	pumpdump.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	pumpdump.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)

	return pumpdump
}

func (pumpdump *PumpDump) Start() error {
	return nil
}

func (pumpdump *PumpDump) State() engine.State {
	return engine.READY
}

func (pumpdump *PumpDump) Tick() error {
	select {
	case <-pumpdump.ctx.Done():
		return pumpdump.ctx.Err()
	case value := <-pumpdump.subscribers["symbols"].Incoming:
		for symbol, pair := range value.Value.(map[string]*asset.Pair) {
			if pair != nil {
				pumpdump.symbols.Store(symbol, NewPumpSymbol(*pair))
			}
		}

		pumpdump.publishPulse()
	case value := <-pumpdump.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		raw, ok := pumpdump.symbols.Load(row.Symbol)

		if !ok {
			break
		}

		state := raw.(*PumpSymbol)

		if row.Last <= 0 {
			break
		}

		state.lastPrice = row.Last
		state.dailyQuoteVol = row.Volume * row.Last

		if row.Bid > 0 {
			state.bid = row.Bid
		}

		if row.Ask > 0 {
			state.ask = row.Ask
		}

		if _, seen := pumpdump.requested.Load(row.Symbol); seen {
			break
		}

		volumes := make([]float64, 0)

		pumpdump.symbols.Range(func(key, value any) bool {
			symbolState := value.(*PumpSymbol)

			if symbolState.dailyQuoteVol > 0 {
				volumes = append(volumes, symbolState.dailyQuoteVol)
			}

			return true
		})

		if len(volumes) < 2 {
			break
		}

		median := numeric.PercentileSorted(numeric.CopySorted(volumes), 0.5)

		if state.dailyQuoteVol < median {
			break
		}

		pumpdump.requested.Store(row.Symbol, struct{}{})
		pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})

		pumpdump.publishPulse()
	case value := <-pumpdump.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		raw, ok := pumpdump.symbols.Load(tick.Symbol)

		if !ok {
			break
		}

		state := raw.(*PumpSymbol)

		closed, err := state.volumeWindow.Next(
			0,
			float64(tick.Timestamp.UnixNano()),
			tick.Qty,
			state.lastPrice,
		)

		if err != nil {
			errnie.Error(err)
			break
		}

		if closed != state.volumeWindow.Sum() {
			if _, err := state.volumeBaseline.Next(0, closed); err != nil {
				errnie.Error(err)
			}
		}

		if _, seen := pumpdump.requested.Load(tick.Symbol); !seen && closed > state.volumeBaseline.Value() {
			pumpdump.requested.Store(tick.Symbol, struct{}{})
			pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
		}

		if tick.Side == "buy" {
			state.buyPressure = 1
			break
		}

		if tick.Side == "sell" {
			state.buyPressure = -1
		}

		pumpdump.publishPulse()
	case value := <-pumpdump.subscribers["feedback"].Incoming:
		pumpdump.Feedback(value.Value.(engine.PredictionFeedback))
		pumpdump.publishPulse()
	case value := <-pumpdump.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		raw, ok := pumpdump.symbols.Load(delta.Symbol)

		if !ok {
			break
		}

		state := raw.(*PumpSymbol)

		if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
			break
		}

		bid := delta.Bids[0].Price
		ask := delta.Asks[0].Price
		mid := (bid + ask) / 2

		state.bid = bid
		state.ask = ask

		if state.lastPrice <= 0 && mid > 0 {
			state.lastPrice = mid
		}

		if mid > 0 {
			state.spreadBPS = (ask - bid) / mid * 10000
		}

		total := delta.Bids[0].Volume + delta.Asks[0].Volume

		if total > 0 {
			state.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
		}

		pumpdump.publishPulse()
	default:
	}

	return nil
}

func (pumpdump *PumpDump) publishPulse() {
	pumpdump.publishMeasurements()
}

func (pumpdump *PumpDump) publishMeasurements() {
	spikeWaiters := make([]chan *qpool.QValue[any], 0)

	pumpdump.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := pumpdump.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*PumpSymbol)
		spikeWaiters = append(
			spikeWaiters,
			pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
				spike, err := state.volumeSpike.Next(
					0,
					state.volumeWindow.Sum(),
					state.volumeBaseline.Value(),
				)

				if err != nil {
					return nil, err
				}

				if spike <= 1 {
					return nil, nil
				}

				return spikeEntry{symbol: symbol, spike: spike}, nil
			}),
		)

		return true
	})

	spikes := make(map[string]float64)

	for _, waiter := range spikeWaiters {
		value := <-waiter

		if value == nil {
			continue
		}

		if value.Error != nil {
			errnie.Error(value.Error)
			continue
		}

		entry, ok := value.Value.(spikeEntry)

		if !ok {
			continue
		}

		spikes[entry.symbol] = entry.spike
	}

	measureWaiters := make([]chan *qpool.QValue[any], 0)

	for symbol, spike := range spikes {
		raw, ok := pumpdump.symbols.Load(symbol)

		if !ok {
			continue
		}

		state := raw.(*PumpSymbol)
		measureWaiters = append(
			measureWaiters,
			pumpdump.pool.ScheduleFast(pumpdump.ctx, func(ctx context.Context) (any, error) {
				peakSpike, err := peakGate.Next(spike, adaptive.PeerValues(spikes, symbol)...)

				if err != nil {
					return nil, err
				}

				measurement, ok := state.Measure(peakSpike)

				if !ok {
					return nil, nil
				}

				return measurement, nil
			}),
		)
	}

	for _, waiter := range measureWaiters {
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

		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
}

type spikeEntry struct {
	symbol string
	spike  float64
}

func (pumpdump *PumpDump) Close() error {
	pumpdump.cancel()

	return nil
}

func (pumpdump *PumpDump) Source() string {
	return pumpdumpSource
}

func (pumpdump *PumpDump) Measure() iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		spikes := make(map[string]float64)

		pumpdump.symbols.Range(func(key, value any) bool {
			symbol := key.(string)

			if _, subscribed := pumpdump.requested.Load(symbol); !subscribed {
				return true
			}

			state := value.(*PumpSymbol)
			spike, err := state.volumeSpike.Next(
				0,
				state.volumeWindow.Sum(),
				state.volumeBaseline.Value(),
			)

			if err != nil {
				errnie.Error(err)
				return true
			}

			if spike <= 1 {
				return true
			}

			spikes[symbol] = spike

			return true
		})

		for symbol, spike := range spikes {
			raw, ok := pumpdump.symbols.Load(symbol)

			if !ok {
				continue
			}

			state := raw.(*PumpSymbol)
			peakSpike, err := peakGate.Next(spike, adaptive.PeerValues(spikes, symbol)...)

			if err != nil {
				errnie.Error(err)
				continue
			}

			measurement, ok := state.Measure(peakSpike)

			if !ok {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (pumpdump *PumpDump) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != pumpdumpSource || feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	raw, ok := pumpdump.symbols.Load(feedback.Symbol)

	if !ok {
		return
	}

	state := raw.(*PumpSymbol)

	if _, err := state.forecast.Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}
