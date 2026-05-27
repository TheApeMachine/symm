package sentiment

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestSymbolStateCalibratedConfidence(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "A/EUR"})
	before := state.calibratedConfidence(0.5)

	if _, err := state.forecastLearner().Next(0, 0.02, -0.02); err != nil {
		t.Fatalf("forecast feedback: %v", err)
	}

	after := state.calibratedConfidence(0.5)

	if after >= before {
		t.Fatalf("expected losing feedback to lower confidence, before=%v after=%v", before, after)
	}
}
