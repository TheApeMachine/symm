package broker

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

/*
MakerQueueTracker accumulates sell-aggressor volume at or through a resting bid
limit after the simulated post time. Queue ahead is snapshotted once at post
time and never reduced by later bid cancellations.
*/
type MakerQueueTracker struct {
	Symbol                   string
	LimitPrice               float64
	PostedAt                 time.Time
	InitialQueueAheadBaseQty float64
	BidTradeVolume           float64
}

/*
NewMakerQueueTracker binds one resting maker bid to a symbol, limit, post time,
and the visible bid book at post time used to freeze queue ahead.
*/
func NewMakerQueueTracker(
	symbol string,
	limitPrice float64,
	postedAt time.Time,
	postTimeBids []market.BookLevel,
) *MakerQueueTracker {
	return &MakerQueueTracker{
		Symbol:                   symbol,
		LimitPrice:               limitPrice,
		PostedAt:                 postedAt,
		InitialQueueAheadBaseQty: QueueAheadBaseQty(postTimeBids, limitPrice),
	}
}

/*
ObserveTrade adds sell-aggressor volume that hits the bid at or through limitPrice
after PostedAt.
*/
func (tracker *MakerQueueTracker) ObserveTrade(trade market.TradeUpdate) {
	if tracker == nil {
		return
	}

	if trade.Symbol != tracker.Symbol {
		return
	}

	if !tracker.PostedAt.IsZero() && trade.Timestamp.Before(tracker.PostedAt) {
		return
	}

	if !SellAggressorHitsBid(trade, tracker.LimitPrice) {
		return
	}

	tracker.BidTradeVolume += trade.Qty
}

/*
Context returns the current queue snapshot for paper fill simulation.
*/
func (tracker *MakerQueueTracker) Context() MakerQueueContext {
	if tracker == nil {
		return MakerQueueContext{}
	}

	return MakerQueueContext{
		InitialQueueAheadBaseQty: tracker.InitialQueueAheadBaseQty,
		BidTradeVolume:           tracker.BidTradeVolume,
	}
}

/*
SellAggressorHitsBid reports whether trade is a sell-aggressor print at or through
limitPrice (volume that consumes resting bids down to our level).
*/
func SellAggressorHitsBid(trade market.TradeUpdate, limitPrice float64) bool {
	if trade.Side != "sell" {
		return false
	}

	if trade.Price <= 0 || trade.Qty <= 0 || limitPrice <= 0 {
		return false
	}

	return trade.Price <= limitPrice+priceLevelEpsilon(limitPrice)
}
