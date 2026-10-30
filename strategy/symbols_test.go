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
	// 3. sentiment (not book-dependent)
	val.Append([]float64{1.0}, "sentiment", "aggregate", "cohort_still_aligned")

	// Add per-symbol signal for a stale symbol and a live symbol
	val.Append([]float64{0.1}, "sentiment", "STALE", "USD", "cohort_still_aligned")
	val.Append([]float64{0.2}, "sentiment", "LIVE", "USD", "cohort_still_aligned")

	bookStale := func(symbol string) bool {
		return symbol == "STALE/USD"
	}

	bySymbol := SignalSourcesBySymbol(val, bookStale)

	// STALE/USD should NOT have liquidity (book-dependent), but SHOULD have correlation & sentiment
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

	if _, ok := staleSources["sentiment"]; !ok {
		t.Error("expected non-book-dependent sentiment to be present for STALE/USD")
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
