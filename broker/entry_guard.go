package broker

// MaxEntrySpreadPct is the touch spread cap for paper and virtual enters.
const MaxEntrySpreadPct = 4.0

/*
EntryPhysicalDecision is the book-integrity outcome at the desk lot size:
spread cap, then close-now vs OpenDragBudgetPct. Gate and desk must both
use CheckEntryPhysical so entry guards cannot drift.
*/
type EntryPhysicalDecision struct {
	Allowed bool
	Reason  string
}

func CheckEntryPhysical(price *Price, symbol string) EntryPhysicalDecision {
	if price == nil {
		return EntryPhysicalDecision{Allowed: true}
	}

	if spreadPct, ok := price.SpreadPct(symbol); ok && spreadPct != nil {
		if spreadPct.Float64() > MaxEntrySpreadPct {
			return EntryPhysicalDecision{
				Allowed: false,
				Reason:  "blown-out spread: bid-ask spread exceeds 4.0%",
			}
		}
	}

	if closeNow, ok := price.EntryCloseNowPnLPct(symbol); ok && closeNow != nil {
		if budget, ok := price.OpenDragBudgetPct(symbol); ok && budget != nil {
			if closeNow.Float64() < -budget.Float64() {
				return EntryPhysicalDecision{
					Allowed: false,
					Reason:  "entry close-now drag exceeds round-trip budget (would open underwater)",
				}
			}
		}
	}

	return EntryPhysicalDecision{Allowed: true}
}
