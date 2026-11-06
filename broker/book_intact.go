package broker

import (
	"time"

	"github.com/theapemachine/symm/system"
)

/*
BookIntact is the book-oracle input for Observe: cash, backing, tape uptake,
and close-now vs open-drag. Spread and peak are separate Observe args.
*/
func (price *Price) BookIntact(symbol string) bool {
	minCash := system.Cfg.Risk.MinTouchBidCash
	cash, ok := price.TouchBidCash(symbol)

	if !ok || cash == nil || cash.InexactFloat64() < minCash {
		return false
	}

	ratio, ok := price.BackingRatio(symbol)

	if !ok || ratio < system.Cfg.Risk.MinBackingRatio {
		return false
	}

	recent, prior := price.tape.Uptake(symbol, time.Now())
	floor := system.Cfg.Risk.MinTapeNotionalUSD

	if recent < floor || recent <= prior {
		return false
	}

	closeNow, ok := price.EntryCloseNowPnLPct(symbol)

	if !ok || closeNow == nil {
		return false
	}

	cap := system.Cfg.Risk.MaxEntryOpenDragPct

	if budget, ok := price.OpenDragBudgetPct(symbol); ok && budget != nil {
		if b := budget.InexactFloat64(); b > 0 && b < cap {
			cap = b
		}
	}

	if fee := price.Fee(symbol); fee != nil {
		if rate := fee.InexactFloat64(); rate > 0 {
			feeFloor := 2 * rate

			if cap < feeFloor {
				cap = feeFloor
			}
		}
	}

	return closeNow.InexactFloat64() >= -cap
}
