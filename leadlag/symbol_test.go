package leadlag

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestSymbolStateForecastScale(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	before := state.forecastScale()

	if _, err := state.forecastLearner().Next(0, 0.02, -0.02); err != nil {
		t.Fatalf("forecast feedback: %v", err)
	}

	after := state.forecastScale()

	if after >= before {
		t.Fatalf("expected losing feedback to lower scale, before=%v after=%v", before, after)
	}
}
