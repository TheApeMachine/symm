package strategy

import (
	"testing"

	"github.com/theapemachine/nomagique/data"
)

func TestSignalSourcesBySymbolBookStaleFiltering(t *testing.T) {
	val := data.NewValue("workspace", "tick", "aggregate")

	// Add aggregate signals:
	// 1. liquidity (book-dependent)
	val.Append([]float64{0.005}, "liquidity", "aggregate", "market_friction")
	// 2. correlation (not book-dependent)
	val.Append([]float64{0.8}, "correlation", "aggregate", "structural_alignment")
	// 3. hawkes (not book-dependent)
	val.Append([]float64{1.1}, "hawkes", "STALE", "USD", "supercritical_cascade")
	val.Append([]float64{1.2}, "hawkes", "LIVE", "USD", "supercritical_cascade")

	bookStale := func(symbol string) bool {
		return symbol == "STALE/USD"
	}

	bySymbol := SignalSourcesBySymbol(val, bookStale)

	// STALE/USD should NOT have liquidity (book-dependent), but SHOULD have correlation & hawkes
	staleSources := bySymbol["STALE/USD"]
	if staleSources == nil {
		t.Fatal("expected STALE/USD in bySymbol")
	}

	if _, ok := staleSources["liquidity"]; ok {
		t.Error("expected liquidity to be omitted for stale symbol STALE/USD")
	}

	if _, ok := staleSources["correlation"]; !ok {
		t.Error("expected non-book-dependent correlation to be present for STALE/USD")
	}

	if _, ok := staleSources["hawkes"]; !ok {
		t.Error("expected non-book-dependent hawkes to be present for STALE/USD")
	}

	// LIVE/USD SHOULD have liquidity
	liveSources := bySymbol["LIVE/USD"]
	if liveSources == nil {
		t.Fatal("expected LIVE/USD in bySymbol")
	}

	if _, ok := liveSources["liquidity"]; !ok {
		t.Error("expected liquidity to be present for live symbol LIVE/USD")
	}
}
