package broker

import (
	"errors"
	"math"

	"github.com/theapemachine/symm/kraken/market"
)

var ErrMakerQueueNotReady = errors.New("maker queue not ready")

/*
MakerQueueContext carries bid-side book depth and sell-aggressor volume since
the resting bid was posted. BidTradeVolume is base quantity traded at or through
the limit price (sellers hitting the bid).
*/
type MakerQueueContext struct {
	Bids           []market.BookLevel
	BidTradeVolume float64
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
ahead of our resting bid and reached our order size.
*/
func MakerFillReady(queue MakerQueueContext, limitPrice, orderBaseQty float64) bool {
	if limitPrice <= 0 || orderBaseQty <= 0 {
		return false
	}

	ahead := QueueAheadBaseQty(queue.Bids, limitPrice)

	return queue.BidTradeVolume >= ahead+orderBaseQty
}

func priceLevelEpsilon(limitPrice float64) float64 {
	return math.Max(limitPrice*1e-9, 1e-12)
}
