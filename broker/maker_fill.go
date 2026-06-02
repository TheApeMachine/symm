package broker

import (
	"math"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
MakerQueueState tracks resting post-only queue position ahead of one order.
*/
type MakerQueueState struct {
	QueueAhead float64
	ActiveAt   int64
}

func NewMakerQueueState(
	quote Quote,
	side trading.Side,
	limitPrice float64,
	activeAtUnixNano int64,
) MakerQueueState {
	levels := quote.Book.Bids

	if side == trading.Sell {
		levels = quote.Book.Asks
	}

	return MakerQueueState{
		QueueAhead: BookLevelQty(levels, limitPrice),
		ActiveAt:   activeAtUnixNano,
	}
}

func BookLevelQty(levels []market.BookLevel, price float64) float64 {
	for _, level := range levels {
		if level.Price == price {
			return level.Qty
		}
	}

	return 0
}

/*
TradeDepletesMakerQueue reports whether a public trade print can consume queue
ahead of a resting post-only limit at limitPrice.
*/
func TradeDepletesMakerQueue(
	side trading.Side,
	limitPrice float64,
	trade market.TradeUpdate,
) (float64, bool) {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return 0, false
	}

	if side == trading.Buy {
		if trade.Side != "sell" || trade.Price > limitPrice {
			return 0, false
		}

		return trade.Qty, true
	}

	if trade.Side != "buy" || trade.Price < limitPrice {
		return 0, false
	}

	return trade.Qty, true
}

func (state *MakerQueueState) Deplete(depletionQty float64) {
	if depletionQty <= 0 {
		return
	}

	state.QueueAhead -= depletionQty

	if state.QueueAhead < 0 {
		state.QueueAhead = 0
	}
}

func (state *MakerQueueState) Ready() bool {
	return state.QueueAhead <= 0
}

/*
MakerAdverseSelectionBps estimates post-fill drift from the trade that lifted
the resting quote. Resting buys filled by sell aggression pay upward slippage.
*/
func MakerAdverseSelectionBps(
	side trading.Side,
	quote Quote,
	trade market.TradeUpdate,
	limitPrice float64,
) float64 {
	spreadBps := MidSpreadBps(quote) * 2

	if spreadBps <= 0 {
		return 0
	}

	levelQty := BookLevelQty(queueSideLevels(quote, side), limitPrice)

	if levelQty <= 0 {
		return spreadBps
	}

	pressure := math.Min(1, trade.Qty/levelQty)

	return spreadBps * pressure
}

func queueSideLevels(quote Quote, side trading.Side) []market.BookLevel {
	if side == trading.Sell {
		return quote.Book.Asks
	}

	return quote.Book.Bids
}

/*
MakerRestingFillPrice returns the pessimistic maker fill and slippage in bps.
*/
func MakerRestingFillPrice(
	side trading.Side,
	limitPrice float64,
	quote Quote,
	trade market.TradeUpdate,
) (float64, float64) {
	adverseBps := MakerAdverseSelectionBps(side, quote, trade, limitPrice)
	fillPrice := limitPrice

	if side == trading.Buy {
		fillPrice *= 1 + adverseBps/10_000
	} else {
		fillPrice *= 1 - adverseBps/10_000
	}

	return fillPrice, adverseBps
}

/*
ReplayMakerAdverseSlippagePct models maker adverse selection during optimizer
replay when the full trade tape is unavailable.
*/
func ReplayMakerAdverseSlippagePct(
	spreadBPS float64,
	stressMultiplier float64,
) float64 {
	if spreadBPS <= 0 {
		return 0
	}

	halfSpread := spreadBPS / 20_000

	return halfSpread * stressMultiplier
}
