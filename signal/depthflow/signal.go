package depthflow

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal detects multi-level order-book imbalance and depth-weighted flow pressure,
mapping book shape onto the weight-of-the-book perspective (LoadedImbalance /
SpoofTrap / BookThinning / DenseNeutrality). Toxic near-touch walls are excluded
via the shared toxicity tracker before distance-decay weighting.
*/
const rawSubscriberID = "signal/depthflow:raw"

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

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	activate.Boot("signal/depthflow ready")

	return signal
}

func (signal *Signal) state(symbol string) *DepthSymbol {
	stored, _ := signal.symbols.LoadOrStore(symbol, NewDepthSymbol(symbol))
	return stored.(*DepthSymbol)
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TradesChannel:
				for _, trade := range errnie.Does(func() ([]market.TradeUpdate, error) {
					return market.DecodeTrades(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					signal.observeTrade(trade)
				}
			case public.TickerChannel:
				for _, ticker := range errnie.Does(func() ([]market.TickerUpdate, error) {
					return market.DecodeTickers(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					signal.state(ticker.Symbol).FeedTicker(ticker)
				}
			case public.BookChannel:
				for _, delta := range errnie.Does(func() ([]market.Book, error) {
					return market.DecodeBooks(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					signal.state(delta.Symbol).ApplyBook(delta)
					signal.emit(delta.Symbol)
				}
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
		activate.Once("signal/depthflow:measurement")
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
