package broker

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/system"
)

/*
EntryDenial is a desk gate rejection with a UI-ready reason.
*/
type EntryDenial struct {
	Gate   string
	Reason string
}

func (denial *EntryDenial) Error() string {
	return denial.Reason
}

/*
EntryPhysicalDecision is the book-integrity outcome at the desk lot size:
local high, day move / volume, spread, absolute bid cash, backing ratio,
tape uptake, then close-now vs open-drag cap. Gate and desk must both use
CheckEntryPhysical so entry guards cannot drift.
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
		if b := budget.InexactFloat64(); b > 0 && b < cap {
			cap = b
		}
	}

	if fee := price.Fee(symbol); fee != nil {
		if rate := fee.InexactFloat64(); rate > 0 {
			floor := 2 * rate

			if cap < floor {
				cap = floor
			}
		}
	}

	return cap
}

/*
CheckEntryPeak blocks entries when last sits at/above max_entry_peak_frac of
the 24h range. Hard rail for every book — including virtual resonance bypass.
*/
func CheckEntryPeak(price *Price, symbol string) EntryPhysicalDecision {
	if price == nil {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "no price book to vet entry peak",
		}
	}

	cap := system.Cfg.Risk.MaxEntryPeakFrac

	if cap <= 0 || cap >= 1 {
		return EntryPhysicalDecision{Allowed: true}
	}

	frac, ok := price.PeakFrac(symbol)

	if !ok {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "24h range unavailable — cannot clear local-high entry gate",
		}
	}

	if frac >= cap {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"too close to local high: last at %.0f%% of 24h range (cap %.0f%%)",
				frac*100,
				cap*100,
			),
		}
	}

	return EntryPhysicalDecision{Allowed: true}
}

/*
CheckEntryMover blocks flat or illiquid names. Volume and open-to-last day
move are desk rails, not only oracle paint — including virtual bypass.
*/
func CheckEntryMover(price *Price, symbol string) EntryPhysicalDecision {
	if price == nil {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "no price book to vet mover",
		}
	}

	volFloor := system.Cfg.Risk.MinMoverVolumeUSD

	if volFloor > 0 {
		volume := price.VolumeUSD(symbol)

		if volume < volFloor {
			return EntryPhysicalDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"24h volume too thin ($%.0f < $%.0f)",
					volume, volFloor,
				),
			}
		}
	}

	chgFloor := system.Cfg.Risk.MinMoverChangePct

	if chgFloor > 0 {
		change := price.ChangePct(symbol)

		if change < chgFloor {
			return EntryPhysicalDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"day move too flat (%.2f%% < %.2f%%) — liquid-but-flat stays out",
					change, chgFloor,
				),
			}
		}
	}

	return EntryPhysicalDecision{Allowed: true}
}

func CheckEntryPhysical(price *Price, symbol string) EntryPhysicalDecision {
	if price == nil {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason:  "no price book to vet entry drag",
		}
	}

	if peak := CheckEntryPeak(price, symbol); !peak.Allowed {
		return peak
	}

	if mover := CheckEntryMover(price, symbol); !mover.Allowed {
		return mover
	}

	// MaxEntrySpreadPct <= 0 means the spread ceiling is disabled (lab zero =
	// no limit). A zero cap would otherwise veto every positive touch spread.
	spreadCap := system.Cfg.Risk.MaxEntrySpreadPct

	if spreadCap > 0 {
		if spreadPct, ok := price.SpreadPct(symbol); ok && spreadPct != nil {
			if spreadPct.InexactFloat64() > spreadCap {
				return EntryPhysicalDecision{
					Allowed: false,
					Reason: fmt.Sprintf(
						"blown-out spread: bid-ask spread exceeds %.1f%%",
						spreadCap,
					),
				}
			}
		}
	}

	return checkBookPhysical(price, symbol)
}

/*
checkBookPhysical is the non-spread half of CheckEntryPhysical. Spread has its
own oracle pair; book integrity must still share this path with the desk.
*/
func checkBookPhysical(price *Price, symbol string) EntryPhysicalDecision {
	minCash := system.Cfg.Risk.MinTouchBidCash
	cash, ok := price.TouchBidCash(symbol)

	if !ok || cash == nil || cash.InexactFloat64() < minCash {
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

	recent, prior := price.tape.Uptake(symbol, time.Now())
	floor := system.Cfg.Risk.MinTapeNotionalUSD

	if recent < floor || recent <= prior {
		return EntryPhysicalDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"tape not picking up (last window $%.0f vs prior $%.0f, floor $%.0f)",
				recent, prior, floor,
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

	if closeNow.InexactFloat64() < -cap {
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

// BookIntact is book integrity without the spread check (spread has its own oracle).
func (price *Price) BookIntact(symbol string) bool {
	return checkBookPhysical(price, symbol).Allowed
}
