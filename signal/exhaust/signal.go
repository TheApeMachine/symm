package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
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

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) Tick() error {
	symbols := config.System.Symbols
	trades := market.NewTradeSubscription(signal.ctx, symbols...)
	ticks := market.NewTickerSubscription(signal.ctx, symbols...)
	books := market.NewBookSubscription(signal.ctx, config.System.BookDepthLevels, symbols...)

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
				sign := -1.0

				if trade.Side == "buy" {
					sign = 1.0
				}

				signal.history.observe(trade.Symbol, 0, 0, 0, 0, sign, 0, trade.Price)
				signal.emit(trade.Symbol)
			}
		case row, ok := <-ticks:
			if !ok {
				ticks = nil
				continue
			}

			if row != nil {
				signal.history.observe(row.Symbol, 0, 0, 0, 0, 0, 0, row.Last)
				signal.emit(row.Symbol)
			}
		case delta, ok := <-books:
			if !ok {
				books = nil
				continue
			}

			if delta != nil {
				signal.observeBook(*delta)
				signal.emit(delta.Symbol)
			}
		}
	}
}

// observeBook folds one book delta's depth, spread, and imbalance into history.
func (signal *Signal) observeBook(delta market.BookUpdate) {
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
	snapshot, ok := signal.history.snapshot(symbol)

	if !ok {
		return
	}

	measurement, ok := exhaustMeasurement(snapshot)

	if !ok {
		return
	}

	measurement.Symbol = symbol
	measurement.SNR = signal.floor.Score(symbol, measurement.SNR)
	signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
