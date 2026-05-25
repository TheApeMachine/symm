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

var _ engine.Signal = (*PumpDump)(nil)

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
		pumpdump.symbols[update.Symbol].FeedTick(update)
	case value := <-pumpdump.subscribers["trade"].Incoming:
		update := value.Value.(TradeUpdate)
		pumpdump.symbols[update.Symbol].FeedTrade(update)
	case value := <-pumpdump.subscribers["book"].Incoming:
		update := value.Value.(BookUpdate)
		pumpdump.symbols[update.Symbol].FeedBook(update)
	default:
		return pumpdump.scoreAll()
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
		pumpdump.eachMeasurement(yield)
	}
}

func (pumpdump *PumpDump) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != pumpdumpSource {
		return
	}

	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	symbolState := pumpdump.symbols[feedback.Symbol]

	if symbolState == nil {
		return
	}

	symbolState.ApplyFeedback(feedback)
}

func (pumpdump *PumpDump) scoreAll() error {
	pumpdump.eachMeasurement(func(measurement engine.Measurement) bool {
		pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
			Value: measurement,
		})

		return true
	})

	return nil
}

func (pumpdump *PumpDump) eachMeasurement(visit func(engine.Measurement) bool) {
	positiveQuotes := make(map[string]float64, len(pumpdump.symbols))

	for symbol, symbolState := range pumpdump.symbols {
		if symbolState.dailyQuoteVol > 0 {
			positiveQuotes[symbol] = symbolState.dailyQuoteVol
		}
	}

	spikes := make(map[string]float64, len(pumpdump.symbols))

	for symbol, symbolState := range pumpdump.symbols {
		if !symbolState.passesLiquidity(positiveQuotes, symbol) {
			continue
		}

		spike, ok := symbolState.spike()

		if !ok {
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

		if !visit(measurement) {
			return
		}
	}
}
