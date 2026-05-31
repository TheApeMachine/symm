package broker

import (
	"errors"
	"math"

	"github.com/theapemachine/symm/kraken/market"
)

var ErrMakerQueueNotReady = errors.New("maker queue not ready")

/*
MakerQueueContext carries the queue-ahead quantity frozen at post time and
sell-aggressor volume accumulated since. BidTradeVolume is base quantity traded
at or through the limit price (sellers hitting the bid).
*/
type MakerQueueContext struct {
	InitialQueueAheadBaseQty float64
	BidTradeVolume           float64
}

/*
QueueAheadBaseQty sums visible bid quantity at limitPrice. A new limit bid joins
the back of that price level, so this is the queue ahead of our order.
*/
func QueueAheadBaseQty(bids []market.BookLevel, limitPrice float64) float64 {
	if limitPrice <= 0 || len(bids) == 0 {
		return 0
	}

	var ahead float64

	for _, level := range bids {
		if level.Price <= 0 || level.Qty <= 0 {
			continue
		}

		if level.Price > limitPrice+priceLevelEpsilon(limitPrice) {
			continue
		}

		if level.Price < limitPrice-priceLevelEpsilon(limitPrice) {
			break
		}

		ahead += level.Qty
	}

	return ahead
}

/*
MakerFillReady reports whether sell-aggressor volume has consumed the queue
ahead frozen at post time and reached our order size.
*/
func MakerFillReady(queue MakerQueueContext, limitPrice, orderBaseQty float64) bool {
	if limitPrice <= 0 || orderBaseQty <= 0 {
		return false
	}

	return queue.BidTradeVolume >= queue.InitialQueueAheadBaseQty+orderBaseQty
}

func priceLevelEpsilon(limitPrice float64) float64 {
	return math.Max(limitPrice*1e-9, 1e-12)
}
