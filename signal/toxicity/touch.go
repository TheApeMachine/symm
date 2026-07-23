package toxicity

import (
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
touchSnapshot freezes one symbol's best bid/ask so the next cut can attribute
fills and cancellations against the touch that preceded the event clock.
*/
type touchSnapshot struct {
	bidPrice    decimal.Decimal
	askPrice    decimal.Decimal
	bidQuantity float64
	askQuantity float64
	observedAt  time.Time
}

/*
touchSide names the exact resting touch consumed by one public trade.
*/
type touchSide int

const (
	touchSideNone touchSide = iota
	touchSideBid
	touchSideAsk
)

/*
attributeTouchSide maps a trade onto the exact executable bid or ask tick.
Prints away from both touches remain valid trades but are not fabricated as
touch fills; malformed off-lattice prices fail explicitly.
*/
func attributeTouchSide(
	tradePrice decimal.Decimal,
	bidPrice *decimal.Decimal,
	askPrice *decimal.Decimal,
	increment *decimal.Decimal,
) (touchSide, error) {
	if bidPrice == nil || askPrice == nil || increment == nil || increment.Sign() <= 0 {
		return touchSideNone, fmt.Errorf("toxicity: complete touch lattice required")
	}

	tradeTick, err := kraken.PriceTick(tradePrice, *increment)

	if err != nil {
		return touchSideNone, fmt.Errorf("toxicity: trade price off lattice: %w", err)
	}

	bidTick, err := kraken.PriceTick(*bidPrice, *increment)

	if err != nil {
		return touchSideNone, fmt.Errorf("toxicity: bid price off lattice: %w", err)
	}

	askTick, err := kraken.PriceTick(*askPrice, *increment)

	if err != nil {
		return touchSideNone, fmt.Errorf("toxicity: ask price off lattice: %w", err)
	}

	if bidTick >= askTick {
		return touchSideNone, fmt.Errorf("toxicity: uncrossed touch required")
	}

	if tradeTick == askTick {
		return touchSideAsk, nil
	}

	if tradeTick == bidTick {
		return touchSideBid, nil
	}

	return touchSideNone, nil
}

/*
attributeTouchFill assigns one trade to at most one live touch side.
*/
func attributeTouchFill(
	row *symbolEvidence,
	tradePrice decimal.Decimal,
	tradeQty float64,
	bid *book.Level,
	ask *book.Level,
	increment *decimal.Decimal,
) error {
	if bid == nil || ask == nil {
		return fmt.Errorf("toxicity: complete Level3 touch required")
	}

	return attributeTouchPrices(
		row,
		tradePrice,
		tradeQty,
		bid.Price,
		ask.Price,
		increment,
	)
}

/*
attributeTouchPrices credits notional to the exact resting touch that was
executable when the trade occurred. The prior snapshot is authoritative
because Level3 may already contain the post-trade book at planner-cut time.
*/
func attributeTouchPrices(
	row *symbolEvidence,
	tradePrice decimal.Decimal,
	tradeQty float64,
	bidPrice *decimal.Decimal,
	askPrice *decimal.Decimal,
	increment *decimal.Decimal,
) error {
	side, err := attributeTouchSide(
		tradePrice,
		bidPrice,
		askPrice,
		increment,
	)

	if err != nil {
		return err
	}

	notional := tradePrice.Float64() * tradeQty

	if side == touchSideBid {
		row.fillBid += notional
		row.bidExecuted += tradeQty
	}

	if side == touchSideAsk {
		row.fillAsk += notional
		row.askExecuted += tradeQty
	}

	return nil
}
