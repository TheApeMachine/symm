package trader

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

/*
TestDeciderEntryOnUntradedSymbolUsesPlaybookConfidence reproduces the live
zero-trades failure mode: a batch where some symbols have field/counterfactual
coverage but another story candidate does not. Missing deferred field evidence
must not silently veto the candidate. The playbook already proposed it from
measured signal evidence, and the trader can rank it by that confidence.
*/
func TestDeciderEntryOnUntradedSymbolUsesPlaybookConfidence(t *testing.T) {
	decider := NewDecider()

	// OTHER/USD has full field/counterfactual coverage. QUIET/USD has only the
	// playbook candidate confidence this tick.
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

	if !containsSymbol(symbols, "QUIET/USD") {
		t.Fatalf("entry without deferred field coverage was blocked: chosen=%v rejections=%v", symbols, rejections)
	}
}
