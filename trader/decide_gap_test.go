package trader

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

/*
TestDeciderEntryOnUntradedSymbolIsNotSilentlyDropped reproduces the live
zero-trades failure the green suite never exercised: a batch where SOME symbols
traded this tick (so causal+manifold emitted for them, making the batch
non-empty) but the story also proposed an entry on a symbol that did NOT trade.
Causal and manifold both require trade frames, so that symbol has neither field
nor uplift. choose() takes the `len(fields) > 0` / `len(uplifts) > 0` branches
and silently `continue`s, dropping the entry with no trade and no recorded
reason. Across an illiquid universe this is every tick — the system opens zero
trades while every unit test passes.
*/
func TestDeciderEntryOnUntradedSymbolIsNotSilentlyDropped(t *testing.T) {
	decider := NewDecider()

	// OTHER/USD traded: full signal coverage. QUIET/USD did not trade, so no
	// causal and no manifold measurement exists for it this tick.
	measurements := []*datura.Artifact{
		manifoldMeasurement("OTHER/USD", 0.8, 0.7, 0.1, 0.05),
		causalMeasurement("OTHER/USD", 1.0),
	}

	actions := []*datura.Artifact{
		candidate("OTHER/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("QUIET/USD", logic.SideBuy, logic.ActionMarket, 0.9),
	}

	chosen, rejections := decider.choose(measurements, actions, nil)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	// The traded symbol with full signal coverage is admitted.
	if !containsSymbol(symbols, "OTHER/USD") {
		t.Fatalf("fully-covered entry was dropped: chosen=%v", symbols)
	}

	// The untraded symbol is correctly vetoed (no causal/field to price it) —
	// but NOT silently. It must come back as an explained verdict so the funnel
	// reports the missing data source instead of vanishing.
	reason := ""
	for _, rejection := range rejections {
		if rejection.symbol == "QUIET/USD" {
			reason = rejection.reason
		}
	}

	if reason == "" {
		t.Fatalf(
			"entry on an untraded symbol was silently dropped with no recorded reason (the zero-trades visibility bug); rejections=%v",
			rejections,
		)
	}
}
