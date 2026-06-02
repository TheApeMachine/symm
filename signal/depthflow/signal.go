package depthflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
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

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TradesChannel:
				trades, err := market.DecodeTrades(&envelope)

				if err != nil {
					return fmt.Errorf("depthflow: decode trades: %w", err)
				}

				for _, trade := range trades {
					if err := signal.observeTrade(trade); err != nil {
						return fmt.Errorf("depthflow: observe trade %s: %w", trade.Symbol, err)
					}
				}
			case public.TickerChannel:
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					return fmt.Errorf("depthflow: decode tickers: %w", err)
				}

				for _, ticker := range tickers {
					state, err := signal.state(ticker.Symbol)

					if err != nil {
						return fmt.Errorf("depthflow: state %s: %w", ticker.Symbol, err)
					}

					state.FeedTicker(ticker)
				}
			case public.BookChannel:
				books, err := market.DecodeBooks(&envelope)

				if err != nil {
					return fmt.Errorf("depthflow: decode books: %w", err)
				}

				for _, delta := range books {
					state, err := signal.state(delta.Symbol)

					if err != nil {
						return fmt.Errorf("depthflow: state %s: %w", delta.Symbol, err)
					}

					state.ApplyBook(delta)

					if err := signal.emit(delta.Symbol); err != nil {
						return fmt.Errorf("depthflow: emit %s: %w", delta.Symbol, err)
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

	activate.Once("signal/depthflow:measurement")
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
