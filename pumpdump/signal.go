package pumpdump

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
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
	liquidityGate = adaptive.NewBelowMedian()
	peakGate      = adaptive.NewPeak()
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
}

var (
	_ engine.System = (*PumpDump)(nil)
	_ engine.Signal = (*PumpDump)(nil)
)

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
	}

	for _, channel := range []string{"symbols", "tick", "trade", "book"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		pumpdump.subscribers[channel] = group.Subscribe("pumpdump:"+channel, 128)
	}

	pumpdump.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

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
		update := value.Value.(TickUpdate)
		state := pumpdump.symbols[update.Symbol]
		state.lastPrice = update.Last
		state.dailyQuoteVol = update.VolumeBase * update.Last
	case value := <-pumpdump.subscribers["trade"].Incoming:
		update := value.Value.(TradeUpdate)
		state := pumpdump.symbols[update.Symbol]
		closed, err := state.volumeWindow.Next(
			0,
			float64(update.UpdatedAt.UnixNano()),
			update.BatchVolume,
			state.lastPrice,
		)

		if err != nil {
			return nil
		}

		if closed != state.volumeWindow.Sum() {
			_, _ = state.volumeBaseline.Next(0, closed)
		}

		if update.BuyPressure > 0 {
			state.buyPressure = update.BuyPressure
		}
	case value := <-pumpdump.subscribers["book"].Incoming:
		update := value.Value.(BookUpdate)
		state := pumpdump.symbols[update.Symbol]
		state.spreadBPS = update.SpreadBPS
		state.imbalance = update.Imbalance
	default:
		for measurement := range pumpdump.Measure() {
			pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
				Value: measurement,
			})
		}
	}

	return nil
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
		positiveQuotes := make(map[string]float64, len(pumpdump.symbols))

		for symbol, state := range pumpdump.symbols {
			if state.dailyQuoteVol > 0 {
				positiveQuotes[symbol] = state.dailyQuoteVol
			}
		}

		spikes := make(map[string]float64, len(pumpdump.symbols))

		for symbol, state := range pumpdump.symbols {
			if state.dailyQuoteVol > 0 {
				if len(positiveQuotes) < 1 {
					continue
				}

				liquid, err := liquidityGate.Next(
					state.dailyQuoteVol,
					adaptive.PeerValues(positiveQuotes, symbol)...,
				)

				if err != nil || liquid <= 0 {
					continue
				}
			}

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
}
