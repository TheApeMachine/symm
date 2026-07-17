package toxicity

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
touchSide names which best touch level absorbed a trade when attribution is
unambiguous within the symbol's price increment lattice.
*/
type touchSide int

const (
	touchSideNone touchSide = iota
	touchSideBid
	touchSideAsk
)

/*
touchToleranceTicks returns the maximum tick distance that still counts as a
touch fill. One live price increment maps to one tick on Kraken's lattice.
*/
func touchToleranceTicks(increment *decimal.Decimal) int64 {
	if increment == nil || increment.Sign() <= 0 {
		return 0
	}

	return 1
}

/*
tickDistanceFromTouch measures how many ticks separate tradePrice from
touchPrice after both prices are normalized to increment.
*/
func tickDistanceFromTouch(
	tradePrice decimal.Decimal,
	touchPrice *decimal.Decimal,
	increment decimal.Decimal,
) (int64, bool) {
	if touchPrice == nil {
		return 0, false
	}

	tradeTick, tradeErr := kraken.PriceTick(tradePrice, increment)
	touchTick, touchErr := kraken.PriceTick(*touchPrice, increment)

	if tradeErr != nil || touchErr != nil {
		return 0, false
	}

	delta := tradeTick - touchTick

	if delta < 0 {
		delta = -delta
	}

	return delta, true
}

/*
sideWithinTouch reports whether tradePrice matches touchPrice within tolerance
ticks, falling back to exact equality when either price is off the increment
lattice.
*/
func sideWithinTouch(
	tradePrice decimal.Decimal,
	touchPrice *decimal.Decimal,
	increment decimal.Decimal,
	tolerance int64,
) bool {
	if touchPrice == nil {
		return false
	}

	distance, ok := tickDistanceFromTouch(tradePrice, touchPrice, increment)

	if ok {
		return distance <= tolerance
	}

	return tradePrice.Cmp(touchPrice) == 0
}

/*
withinTouchTolerance reports whether tradePrice is within one price increment
of touchPrice on the symbol's tick lattice.
*/
func withinTouchTolerance(
	tradePrice decimal.Decimal,
	touchPrice *decimal.Decimal,
	increment *decimal.Decimal,
) bool {
	if touchPrice == nil {
		return false
	}

	if increment == nil || increment.Sign() <= 0 {
		return tradePrice.Cmp(touchPrice) == 0
	}

	return sideWithinTouch(
		tradePrice,
		touchPrice,
		*increment,
		touchToleranceTicks(increment),
	)
}

/*
attributeTouchSide assigns a trade to bid or ask when it lies within one tick
of a touch. Equal distances prefer ask so a trade at the exact mid of a
one-tick spread credits the offer side once.
*/
func attributeTouchSide(
	tradePrice decimal.Decimal,
	bidPrice *decimal.Decimal,
	askPrice *decimal.Decimal,
	increment *decimal.Decimal,
) touchSide {
	if increment == nil || increment.Sign() <= 0 {
		bidExact := bidPrice != nil && tradePrice.Cmp(bidPrice) == 0
		askExact := askPrice != nil && tradePrice.Cmp(askPrice) == 0

		if bidExact && askExact {
			return touchSideAsk
		}

		if askExact {
			return touchSideAsk
		}

		if bidExact {
			return touchSideBid
		}

		return touchSideNone
	}

	tolerance := touchToleranceTicks(increment)
	bidWithin := sideWithinTouch(tradePrice, bidPrice, *increment, tolerance)
	askWithin := sideWithinTouch(tradePrice, askPrice, *increment, tolerance)

	if !bidWithin && !askWithin {
		return touchSideNone
	}

	if bidWithin && !askWithin {
		return touchSideBid
	}

	if askWithin && !bidWithin {
		return touchSideAsk
	}

	bidDistance, bidOk := tickDistanceFromTouch(tradePrice, bidPrice, *increment)
	askDistance, askOk := tickDistanceFromTouch(tradePrice, askPrice, *increment)

	if bidOk && askOk {
		if bidDistance < askDistance {
			return touchSideBid
		}

		if askDistance < bidDistance {
			return touchSideAsk
		}
	}

	return touchSideAsk
}

/*
attributeTouchFill assigns one trade to at most one touch side and updates
the symbol evidence counters used by touchHonesty.
*/
func attributeTouchFill(
	row *symbolEvidence,
	tradePrice decimal.Decimal,
	tradeQty float64,
	bid *book.Level,
	ask *book.Level,
	increment *decimal.Decimal,
) {
	if bid == nil || ask == nil {
		return
	}

	side := attributeTouchSide(tradePrice, bid.Price, ask.Price, increment)
	volume := decimal.NewFromFloat64(tradeQty)

	if side == touchSideBid {
		row.fillBid = zeroed(row.fillBid).Add(bid.Price.Mul(volume))
		row.bidExecuted += tradeQty
	}

	if side == touchSideAsk {
		row.fillAsk = zeroed(row.fillAsk).Add(ask.Price.Mul(volume))
		row.askExecuted += tradeQty
	}
}
