package pumpdump

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const tradeWindow = 5 * time.Minute

var moveClassifier, _ = adaptive.NewClassifier(
	[]float64{-0.001, 0.001},
	[]float64{0, 1, 2},
	[]string{"dump", "precursor", "actual_pump"},
)

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
Watch list comes from symbols; market data from tick, trade, and book.
*/
type PumpDump struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     map[string]*PumpSymbol
	peak        *adaptive.Peak
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
		peak:        adaptive.NewPeak(),
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
		update := value.Value.(engine.TickUpdate)
		pumpdump.symbols[update.Symbol].FeedTick(update)
	case value := <-pumpdump.subscribers["trade"].Incoming:
		update := value.Value.(engine.TradeUpdate)
		pumpdump.symbols[update.Symbol].FeedTrade(update)
	case value := <-pumpdump.subscribers["book"].Incoming:
		update := value.Value.(engine.BookUpdate)
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

func (pumpdump *PumpDump) scoreAll() error {
	quotes := make(map[string]float64, len(pumpdump.symbols))

	for symbol, sym := range pumpdump.symbols {
		quotes[symbol] = sym.dailyQuoteVol
	}

	spikes := make(map[string]float64, len(pumpdump.symbols))

	for symbol, sym := range pumpdump.symbols {
		if sym.dailyQuoteVol > 0 &&
			!engine.PassesBelowMedianLiquidity(sym.dailyQuoteVol, quotes, symbol, 1) {
			continue
		}

		spike := errnie.Does(func() (float64, error) {
			return sym.volumeSpike.Next(0, sym.volumeWindow.Sum(), sym.volumeBaseline.Value())
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		if spike > 1 {
			spikes[symbol] = spike
		}
	}

	for symbol, spike := range spikes {
		peakSpike := errnie.Does(func() (float64, error) {
			return pumpdump.peak.Next(spike, adaptive.PeerValues(spikes, symbol)...)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		if measurement, ok := pumpdump.symbols[symbol].Measure(peakSpike); ok {
			pumpdump.broadcasts["measurements"].Send(&qpool.QValue[any]{
				Value: measurement,
			})
		}
	}

	return nil
}
