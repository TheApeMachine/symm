package depthflow

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	signalpool "github.com/theapemachine/symm/signal"
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
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/depthflow ready", "signal/depthflow")

	return signal
}

func (signal *Signal) state(symbol string) (*DepthSymbol, error) {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*DepthSymbol), nil
	}

	created, err := NewDepthSymbol(symbol)

	if err != nil {
		return nil, err
	}

	stored, _ := signal.symbols.LoadOrStore(symbol, created)

	return stored.(*DepthSymbol), nil
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

			sm, ok := signalpool.SocketMessageFromValue(message.Value)

			if !ok {
				continue
			}

			switch sm.Channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)

				for _, trade := range trades {
					if err := signal.observeTrade(trade); err != nil {
						errnie.Error(err, "depthflow: observe trade %s", trade.Symbol)
						continue
					}
				}
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)

				for _, ticker := range tickers {
					state, err := signal.state(ticker.Symbol)

					if err != nil {
						errnie.Error(err, "depthflow: state %s", ticker.Symbol)
						continue
					}

					state.FeedTicker(ticker)
				}
			case public.BookChannel:
				books := signalpool.GetBooks(sm)

				for _, delta := range books {
					state, err := signal.state(delta.Symbol)

					if err != nil {
						errnie.Error(err, "depthflow: state %s", delta.Symbol)
						continue
					}

					state.ApplyBook(delta)

					if err := signal.emit(delta.Symbol); err != nil {
						errnie.Error(err, "depthflow: emit %s", delta.Symbol)
						continue
					}
				}
			}
		}
	}
}

// observeTrade folds one trade's aggressor side into depth-weighted pressure and
// emits the symbol's reading.
func (signal *Signal) observeTrade(trade market.TradeUpdate) error {
	sign := -1.0

	if trade.Side == "buy" {
		sign = 1.0
	}

	state, err := signal.state(trade.Symbol)

	if err != nil {
		return err
	}

	if _, err := state.PushTradePressure(sign); err != nil {
		return err
	}

	return signal.emit(trade.Symbol)
}

func (signal *Signal) emit(symbol string) error {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	measurement, standout, err := raw.(*DepthSymbol).Measure()

	if err != nil {
		return err
	}

	if measurement.Source == perspectives.SourceNone {
		return nil
	}

	measurement.Symbol = symbol
	if err := perspectives.AssignCategorySNR(
		&measurement, signal.floor, standout,
	); err != nil {
		return err
	}

	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
			Value: map[string]any{
				"chart":      "gauge",
				"source":     measurement.Source.String(),
				"confidence": measurement.Confidence,
				"snr":        measurement.SNR,
			},
		})
	}

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
