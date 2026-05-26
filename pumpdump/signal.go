package pumpdump

import (
	"context"
	"iter"
	"time"

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
	moveClassifier, _ = adaptive.NewClassifier(
		[]float64{-0.001, 0.001},
		[]float64{0, 1, 2},
		[]string{"dump", "precursor", "actual_pump"},
	)
	peakGate = adaptive.NewPeak()
)

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*PumpSymbol
	requested   map[string]struct{}
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
		symbols:     make(map[string]*PumpSymbol),
		requested:   make(map[string]struct{}),
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
				pumpdump.symbols[symbol] = NewPumpSymbol(*pair)
			}
		}
	case value := <-pumpdump.subscribers["tick"].Incoming:
		row := value.Value.(market.TickerRow)
		state := pumpdump.symbols[row.Symbol]

		if state == nil || row.Last <= 0 {
			break
		}

		state.lastPrice = row.Last
		state.dailyQuoteVol = row.Volume * row.Last

		if _, seen := pumpdump.requested[row.Symbol]; seen {
			break
		}

		volumes := make([]float64, 0, len(pumpdump.symbols))

		for _, symbolState := range pumpdump.symbols {
			if symbolState.dailyQuoteVol > 0 {
				volumes = append(volumes, symbolState.dailyQuoteVol)
			}
		}

		if len(volumes) < 2 {
			break
		}

		median := numeric.PercentileSorted(numeric.CopySorted(volumes), 0.5)

		if state.dailyQuoteVol < median {
			break
		}

		pumpdump.requested[row.Symbol] = struct{}{}
		pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{row.Symbol}})
	case value := <-pumpdump.subscribers["trade"].Incoming:
		tick := value.Value.(trade.Data)
		state := pumpdump.symbols[tick.Symbol]

		if state == nil {
			break
		}

		closed, err := state.volumeWindow.Next(
			0,
			float64(tick.Timestamp.UnixNano()),
			tick.Qty,
			state.lastPrice,
		)

		if err != nil {
			break
		}

		if closed != state.volumeWindow.Sum() {
			_, _ = state.volumeBaseline.Next(0, closed)
		}

		if _, seen := pumpdump.requested[tick.Symbol]; !seen && closed > state.volumeBaseline.Value() {
			pumpdump.requested[tick.Symbol] = struct{}{}
			pumpdump.broadcasts["subscriptions"].Send(&qpool.QValue[any]{Value: []string{tick.Symbol}})
		}

		if tick.Side == "buy" {
			state.buyPressure = 1
			break
		}

		if tick.Side == "sell" {
			state.buyPressure = -1
		}
	case value := <-pumpdump.subscribers["feedback"].Incoming:
		pumpdump.Feedback(value.Value.(engine.PredictionFeedback))
	case value := <-pumpdump.subscribers["book"].Incoming:
		delta := value.Value.(market.BookLevelsDelta)
		state := pumpdump.symbols[delta.Symbol]

		if state == nil {
			break
		}

		if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
			break
		}

		bid := delta.Bids[0].Price
		ask := delta.Asks[0].Price
		mid := (bid + ask) / 2

		if mid > 0 {
			state.spreadBPS = (ask - bid) / mid * 10000
		}

		total := delta.Bids[0].Volume + delta.Asks[0].Volume

		if total > 0 {
			state.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
		}
	default:
	}

	pumpdump.publishPulse()

	return nil
}

func (pumpdump *PumpDump) publishPulse() {
	for measurement := range pumpdump.Measure() {
		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})
	}
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
		spikes := make(map[string]float64, len(pumpdump.symbols))

		for symbol, state := range pumpdump.symbols {
			spike, err := state.volumeSpike.Next(
				0,
				state.volumeWindow.Sum(),
				state.volumeBaseline.Value(),
			)

			if err != nil || spike <= 1 {
				continue
			}

			spikes[symbol] = spike
		}

		for symbol, spike := range spikes {
			peakSpike, err := peakGate.Next(spike, adaptive.PeerValues(spikes, symbol)...)

			if err != nil {
				continue
			}

			measurement, ok := pumpdump.symbols[symbol].Measure(peakSpike)

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

	state := pumpdump.symbols[feedback.Symbol]

	if state == nil {
		return
	}

	_, _ = state.forecast.Next(0, feedback.PredictedReturn, feedback.ActualReturn)
	state.confidence.ApplyFeedback(feedback)
}
