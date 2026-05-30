package toxicity

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Toxicity tracks executed-flow book quality and publishes toxicity perspective
measurements while feeding IsToxic for depthflow and fluid.
*/
type Toxicity struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	tracker      *Tracker
	measurements *qpool.BroadcastGroup
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	tox := &Toxicity{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		tracker: Default(),
	}
	tox.measurements = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return tox
}

/*
Tick joins the live trade tape, ticker, and L2 book onto the shared Tracker.
*/
func (tox *Toxicity) Tick() error {
	symbols := config.System.Symbols
	trades := market.NewTradeSubscription(tox.ctx, symbols...)
	ticks := market.NewTickerSubscription(tox.ctx, symbols...)
	books := market.NewBookSubscription(tox.ctx, config.System.BookDepthLevels, symbols...)

	for {
		select {
		case <-tox.ctx.Done():
			return tox.ctx.Err()
		case trade, ok := <-trades:
			if !ok {
				trades = nil

				continue
			}

			if trade != nil {
				tox.observeTrade(*trade)
			}
		case row, ok := <-ticks:
			if !ok {
				ticks = nil

				continue
			}

			if row != nil {
				tox.tracker.ObserveMid(row.Symbol, market.Pair{}, midOf(*row))
				tox.publishMeasurement(row.Symbol, row.Last)
			}
		case update, ok := <-books:
			if !ok {
				books = nil

				continue
			}

			if update != nil {
				tox.observeBook(*update)
				tox.publishMeasurement(update.Symbol, 0)
			}
		}
	}
}

func (tox *Toxicity) observeTrade(trade market.TradeUpdate) {
	tox.tracker.ObserveTrade(trade.Symbol, market.Pair{}, trade.Price, trade.Qty, trade.Timestamp)
}

func (tox *Toxicity) observeBook(update market.BookUpdate) {
	now := time.Now()

	for _, level := range update.Bids {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideBid, level.Price, level.Qty, now)
	}

	for _, level := range update.Asks {
		tox.tracker.ApplyBookLevel(update.Symbol, market.Pair{}, SideAsk, level.Price, level.Qty, now)
	}
}

func (tox *Toxicity) publishMeasurement(symbol string, last float64) {
	now := time.Now()
	measurement, ok := tox.tracker.Measure(symbol, now)

	if !ok {
		return
	}

	measurement.Symbol = symbol
	measurement.Last = last

	tox.measurements.Send(&qpool.QValue[any]{Value: measurement})
}

func (tox *Toxicity) Close() error {
	tox.cancel()
	return nil
}

func midOf(row market.TickerUpdate) float64 {
	if row.Bid > 0 && row.Ask > 0 {
		return (row.Bid + row.Ask) / 2
	}

	return row.Last
}
