package exhaust

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal tracks book/trade microstructure decay and classifies the dominant
exhaustion mode onto the exhaustion perspective. Exit timing itself is decided
at the perspective layer (ActionStopLoss / ActionExit); this signal only
publishes the classified reading.
*/
const rawSubscriberID = "signal/exhaust:raw"

type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	history     *historyStore
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
		history:     newHistoryStore(),
		floor:       adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	activate.Boot("signal/exhaust ready")

	return signal
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
					return fmt.Errorf("exhaust: decode trades: %w", err)
				}

				for _, trade := range trades {
					sign := -1.0

					if trade.Side == "buy" {
						sign = 1.0
					}

					signal.history.observe(trade.Symbol, 0, 0, 0, 0, sign, 0, trade.Price)

					if err := signal.emit(trade.Symbol); err != nil {
						return fmt.Errorf("exhaust: emit %s: %w", trade.Symbol, err)
					}
				}
			case public.TickerChannel:
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					return fmt.Errorf("exhaust: decode tickers: %w", err)
				}

				for _, ticker := range tickers {
					signal.history.observe(ticker.Symbol, 0, 0, 0, 0, 0, 0, ticker.Last)

					if err := signal.emit(ticker.Symbol); err != nil {
						return fmt.Errorf("exhaust: emit %s: %w", ticker.Symbol, err)
					}
				}
			case public.BookChannel:
				books, err := market.DecodeBooks(&envelope)

				if err != nil {
					return fmt.Errorf("exhaust: decode books: %w", err)
				}

				for _, delta := range books {
					signal.observeBook(delta)

					if err := signal.emit(delta.Symbol); err != nil {
						return fmt.Errorf("exhaust: emit %s: %w", delta.Symbol, err)
					}
				}
			}
		}
	}
}

// observeBook folds one book delta's depth, spread, and imbalance into history.
func (signal *Signal) observeBook(delta market.Book) {
	bidDepth := 0.0
	askDepth := 0.0

	for _, level := range delta.Bids {
		bidDepth += level.Qty
	}

	for _, level := range delta.Asks {
		askDepth += level.Qty
	}

	spreadBPS := 0.0
	imbalance := 0.0

	if len(delta.Bids) > 0 && len(delta.Asks) > 0 {
		bid := delta.Bids[0].Price
		ask := delta.Asks[0].Price
		mid := (bid + ask) / 2

		if mid > 0 {
			spreadBPS = (ask - bid) / mid * 10000
		}

		total := delta.Bids[0].Qty + delta.Asks[0].Qty

		if total > 0 {
			imbalance = (delta.Bids[0].Qty - delta.Asks[0].Qty) / total
		}
	}

	signal.history.observe(delta.Symbol, bidDepth, askDepth, bidDepth+askDepth, spreadBPS, 0, imbalance, 0)
}

// emit publishes the exhaustion reading for the one symbol an event touched.
func (signal *Signal) emit(symbol string) error {
	measurement, standout, err := signal.history.measure(symbol)

	if err != nil {
		return err
	}

	if measurement.Source == perspectives.SourceNone {
		return nil
	}

	measurement.Symbol = symbol
	if err := perspectives.AssignCategorySNR(&measurement, signal.floor, standout); err != nil {
		return err
	}

	activate.Once("signal/exhaust:measurement")
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
