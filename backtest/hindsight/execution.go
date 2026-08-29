package hindsight

import (
	"time"
)

/*
ExecutableLeg is the quantity-constrained counterfactual economics of one leg.
It separates what the observed price path offered (theoretical) from what the
recorded quantity could actually capture through historical L3 depth
(executable). Every executable number is derived by walking the reconstructed
book — never by trusting a top-of-book quote alone.
*/
type ExecutableLeg struct {
	Symbol string    `json:"symbol"`
	BuyAt  time.Time `json:"buyAt"`
	SellAt time.Time `json:"sellAt"`

	// Theoretical economics: the price path with no size claim.
	TheoreticalBuyPrice  float64 `json:"theoreticalBuyPrice"`
	TheoreticalSellPrice float64 `json:"theoreticalSellPrice"`
	TheoreticalReturn    float64 `json:"theoreticalReturn"`

	// The counterfactual size defended from the recorded decision state.
	RequestedQty      float64 `json:"requestedQty"`
	RequestedNotional float64 `json:"requestedNotional"`

	// Executable entry through historical ask depth.
	ExecutableEntryQty   float64 `json:"executableEntryQty"`
	ExecutableEntryVWAP  float64 `json:"executableEntryVWAP"`
	ExecutableEntryValue float64 `json:"executableEntryValue"`
	ExecutableEntryFees  float64 `json:"executableEntryFees"`
	EntryImpact          float64 `json:"entryImpact"`

	// Executable exit through historical bid depth.
	ExecutableExitQty   float64 `json:"executableExitQty"`
	ExecutableExitVWAP  float64 `json:"executableExitVWAP"`
	ExecutableExitValue float64 `json:"executableExitValue"`
	ExecutableExitFees  float64 `json:"executableExitFees"`
	ExitImpact          float64 `json:"exitImpact"`

	// Whether the full requested quantity was executable on both legs.
	FullyExecutable  bool    `json:"fullyExecutable"`
	ExecutableReturn float64 `json:"executableReturn"`
	ExecutablePnL    float64 `json:"executablePnL"`
}

/*
ExecutableCounterfactual prices one leg against the reconstructed historical
books at its entry and exit times. It returns ok=false (executable economics
undefined) when the book was uninitialized at either boundary, when the
requested quantity could not be defended, or when either walk could not fill
the full requested lot — the round trip is not fully executable.
*/
func ExecutableCounterfactual(
	store *BookStore,
	leg Leg,
	requestedQty float64,
	feeRate float64,
) (ExecutableLeg, bool) {
	if store == nil || requestedQty <= 0 {
		return ExecutableLeg{}, false
	}

	entryBook, entryReady := store.BookAt(leg.Symbol, leg.BuyAt)

	if !entryReady {
		return ExecutableLeg{}, false
	}

	exitBook, exitReady := store.BookAt(leg.Symbol, leg.SellAt)

	if !exitReady {
		return ExecutableLeg{}, false
	}

	entryWalk := WalkAsks(entryBook.Asks, requestedQty)
	exitWalk := WalkBids(exitBook.Bids, requestedQty)

	fullyExecutable := entryWalk.FilledQty >= requestedQty &&
		exitWalk.FilledQty >= requestedQty

	if !fullyExecutable {
		// Insufficient depth on either leg leaves the round trip undefined:
		// it is not a question of choosing a smaller size, and best-of-book
		// is never a substitute for the missing depth.
		return ExecutableLeg{}, false
	}

	entryVWAP := entryWalk.VWAP
	exitVWAP := exitWalk.VWAP
	entryValue := entryWalk.Gross
	exitValue := exitWalk.Gross

	entryFees := entryValue * feeRate
	exitFees := exitValue * feeRate

	out := ExecutableLeg{
		Symbol:               leg.Symbol,
		BuyAt:                leg.BuyAt,
		SellAt:               leg.SellAt,
		TheoreticalBuyPrice:  leg.BuyPrice,
		TheoreticalSellPrice: leg.SellPrice,
		TheoreticalReturn:    leg.GrossProfitPct,
		RequestedQty:         requestedQty,
		RequestedNotional:    entryValue,
		ExecutableEntryQty:   entryWalk.FilledQty,
		ExecutableEntryVWAP:  entryVWAP,
		ExecutableEntryValue: entryValue,
		ExecutableEntryFees:  entryFees,
		EntryImpact:          entryVWAP - leg.BuyPrice,
		ExecutableExitQty:    exitWalk.FilledQty,
		ExecutableExitVWAP:   exitVWAP,
		ExecutableExitValue:  exitValue,
		ExecutableExitFees:   exitFees,
		ExitImpact:           leg.SellPrice - exitVWAP,
		FullyExecutable:      true,
	}

	if entryValue > 0 {
		// Executable return: net proceeds minus net cost over entry value.
		netEntry := entryValue + entryFees
		netExit := exitValue - exitFees
		out.ExecutableReturn = (netExit - netEntry) / entryValue
		out.ExecutablePnL = netExit - netEntry
	}

	return out, true
}
