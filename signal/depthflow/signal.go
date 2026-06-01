package depthflow

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal detects multi-level order-book imbalance and depth-weighted flow pressure,
mapping book shape onto the weight-of-the-book perspective (LoadedImbalance /
SpoofTrap / BookThinning / DenseNeutrality). Toxic near-touch walls are excluded
via the shared toxicity tracker before distance-decay weighting.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	floor       *adaptive.SNRField
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		floor:       adaptive.NewSNRField(),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) state(symbol string) *DepthSymbol {
	stored, _ := signal.symbols.LoadOrStore(symbol, NewDepthSymbol(symbol))
	return stored.(*DepthSymbol)
}

func (signal *Signal) Tick() error {
	symbols := viper.GetViper().GetStringSlice("market.symbols")
	trades := market.NewTradeSubscription(signal.ctx, symbols...)
	ticks := market.NewTickerSubscription(signal.ctx, symbols...)
	books := market.NewBookSubscription(signal.ctx, viper.GetViper().GetInt("market.book_depth_levels"), symbols...)

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case trade, ok := <-trades:
			if !ok {
				trades = nil
				continue
			}

			if trade != nil {
				signal.observeTrade(*trade)
			}
		case row, ok := <-ticks:
			if !ok {
				ticks = nil
				continue
			}

			if row != nil {
				signal.state(row.Symbol).FeedTicker(*row)
			}
		case delta, ok := <-books:
			if !ok {
				books = nil
				continue
			}

			if delta != nil {
				signal.state(delta.Symbol).ApplyBook(*delta)
				signal.emit(delta.Symbol)
			}
		}
	}
}

// observeTrade folds one trade's aggressor side into depth-weighted pressure and
// emits the symbol's reading.
func (signal *Signal) observeTrade(trade market.TradeUpdate) {
	sign := -1.0

	if trade.Side == "buy" {
		sign = 1.0
	}

	if _, err := signal.state(trade.Symbol).PushTradePressure(sign); err != nil {
		errnie.Error(err)
	}

	signal.emit(trade.Symbol)
}

func (signal *Signal) emit(symbol string) {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return
	}

	measurement, ok := raw.(*DepthSymbol).Measure()

	if ok {
		measurement.Symbol = symbol
		measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
