package broker

import (
	"fmt"

	"github.com/theapemachine/symm/system"
)

/*
EntryPhysicalDecision is the book-integrity outcome at the desk lot size:
spread, absolute bid cash, backing ratio, tape liveness, then close-now vs
open-drag cap. Gate and desk must both use CheckEntryPhysical so entry
guards cannot drift.
*/
type EntryPhysicalDecision struct {
	Allowed bool
	Reason  string
}

/*
openDragCapPct is the tighter of risk.max_entry_open_drag_pct and the
friction budget, but never below the two-leg taker fee floor — that cost is
unavoidable even with a one-tick spread.
*/
func openDragCapPct(price *Price, symbol string) float64 {
	cap := system.Cfg.Risk.MaxEntryOpenDragPct

	if budget, ok := price.OpenDragBudgetPct(symbol); ok && budget != nil {
		if b := budget.MustFloat64(); b > 0 && b < cap {
			cap = b
		}
	}

	fee := price.feeForTrade(symbol).MustFloat64()

	if fee > 0 {
		floor := 2 * fee

		if cap < floor {
			cap = floor
		}
	}

	return cap
}

func CheckEntryPhysical(price *Price, symbol string) EntryPhysicalDecision {
	if price == nil {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "no price book to vet entry drag",
		}
	}

	spreadCap := system.Cfg.Risk.MaxEntrySpreadPct

	if spreadPct, ok := price.SpreadPct(symbol); ok && spreadPct != nil {
		if spreadPct.MustFloat64() > spreadCap {
			return EntryPhysicalDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"blown-out spread: bid-ask spread exceeds %.1f%%",
					spreadCap,
				),
			}
		}
	}

	minCash := system.Cfg.Risk.MinTouchBidCash
	cash, ok := price.TouchBidCash(symbol)

	if !ok || cash == nil || cash.MustFloat64() < minCash {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"book too thin (<$%.0f on top bids)",
				minCash,
			),
		}
	}

	ratio, ok := price.BackingRatio(symbol)

	if !ok || ratio < system.Cfg.Risk.MinBackingRatio {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"bid support thinner than asks within %.0f%% of mid (ratio %.2f)",
				system.Cfg.Risk.BackingBandFrac*100,
				ratio,
			),
		}
	}

	if price.Tape == nil || !price.Tape.Alive(symbol) {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"tape too quiet (< %d trades in %s)",
				system.Cfg.Risk.MinTapeTrades,
				system.Cfg.Risk.TapeWindow,
			),
		}
	}

	closeNow, ok := price.EntryCloseNowPnLPct(symbol)

	if !ok || closeNow == nil {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "cannot price entry close-now drag at desk lot size",
		}
	}

	cap := openDragCapPct(price, symbol)

	if closeNow.MustFloat64() < -cap {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"entry close-now drag exceeds %.1f%% cap (would open too underwater)",
				cap,
			),
		}
	}

	return EntryPhysicalDecision{Allowed: true}
}
