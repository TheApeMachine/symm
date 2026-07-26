package toxicity

import (
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
attributeTouchSide maps a trade onto the exact executable bid or ask price.
Prints away from both touches remain valid trades but are not fabricated as
touch fills.
*/
func attributeTouchSide(
	tradePrice decimal.Decimal,
	bidPrice *decimal.Decimal,
	askPrice *decimal.Decimal,
) (touchSide, error) {
	if bidPrice == nil || askPrice == nil {
		return touchSideNone, fmt.Errorf("toxicity: complete touch required")
	}

	if bidPrice.Cmp(askPrice) >= 0 {
		return touchSideNone, fmt.Errorf("toxicity: uncrossed touch required")
	}

	if tradePrice.Cmp(askPrice) == 0 {
		return touchSideAsk, nil
	}

	if tradePrice.Cmp(bidPrice) == 0 {
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
) error {
	side, err := attributeTouchSide(tradePrice, bidPrice, askPrice)

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
