package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Signal tracks book/trade microstructure decay and classifies the dominant
exhaustion mode onto the exhaustion perspective. Exit timing itself is decided
at the perspective layer (ActionStopLoss / ActionExit); this signal only
publishes the classified reading.
*/
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

	for _, channel := range []string{"trade", "ticker", "book", "measurements"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(channel, 128)
	}

	return signal
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["trade"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				trade, err := market.DecodeTrade(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				sign := -1.0

				if trade.Side == "buy" {
					sign = 1.0
				}

				signal.history.observe(trade.Symbol, 0, 0, 0, 0, sign, 0, trade.Price)
				signal.emit(trade.Symbol)
			}
		case message := <-signal.subscribers["ticker"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				ticker, err := market.DecodeTicker(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				signal.history.observe(ticker.Symbol, 0, 0, 0, 0, 0, 0, ticker.Last)
				signal.emit(ticker.Symbol)
			}
		case message := <-signal.subscribers["book"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			rows, err := envelope.SplitDataRows()

			if err != nil {
				errnie.Error(err)

				continue
			}

			for _, row := range rows {
				delta, err := market.DecodeBook(row)

				if err != nil {
					errnie.Error(err)

					continue
				}

				signal.observeBook(delta)
				signal.emit(delta.Symbol)
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
// Each symbol's reading is independent, so there is no need to re-measure the
// whole cross-section on every event.
func (signal *Signal) emit(symbol string) {
	measurement, ok := signal.history.measure(symbol)

	if !ok {
		return
	}

	measurement.Symbol = symbol
	measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
