package toxicity

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Toxicity is the executed-flow book-quality service. It feeds the shared Tracker
from the public L2 book joined against the public trade tape, splitting liquidity
removals into fills vs cancels, and (via the package-level IsToxic) lets the
weighted-book readers in depthflow/fluid exclude toxic near-touch walls. It is a
service, not a perspectives source — it emits no measurements.

It consumes the same shared market subscriptions the live signals do
(market.NewTradeSubscription / NewTickerSubscription / NewBookSubscription) rather
than a separate qpool fan-out, so the book quality it tracks is the book the rest of
the engine is actually reading.
*/
type Toxicity struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pool    *qpool.Q
	tracker *Tracker
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	return &Toxicity{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		tracker: Default(),
	}
}

/*
Tick joins the live trade tape, ticker, and L2 book onto the shared Tracker. A trade
is executed flow, a ticker refreshes the mid the tracker prices removals against, and
each book level is a maker action — a zero-quantity level is liquidity leaving, which
the tracker splits into a fill or a cancel.
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
			}
		case update, ok := <-books:
			if !ok {
				books = nil

				continue
			}

			if update != nil {
				tox.observeBook(*update)
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
