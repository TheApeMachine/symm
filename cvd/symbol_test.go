package cvd

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func testPair() asset.Pair {
	return asset.Pair{Wsname: "AAA/EUR", Quote: "EUR"}
}

var testBase = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// feed pushes count trades of unit volume at the given price and side.
func feed(state *CVDSymbol, count int, price float64, takerBuy bool) {
	for range count {
		state.FeedTrade(price, 1, takerBuy, testBase)
	}
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestCVDBelowMinTradesIsSilent(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, cvdMinTrades-1, 100, true)

	if _, ok := state.Measure(testBase.Add(time.Minute)); ok {
		t.Fatalf("expected no measurement below cvdMinTrades (%d)", cvdMinTrades)
	}
}

func TestCVDAbsorptionOnFlatPriceNetBuying(t *testing.T) {
	state := NewCVDSymbol(testPair())
	// 45 taker buys + 5 taker sells, all at 100 => netFraction 0.8, flat price.
	feed(state, 45, 100, true)
	feed(state, 5, 100, false)

	m, ok := state.Measure(testBase.Add(time.Minute))

	if !ok {
		t.Fatal("expected an accumulation measurement")
	}

	if m.Type != engine.Momentum {
		t.Fatalf("expected Momentum type, got %v", m.Type)
	}

	if m.Source != "cvd" || m.Regime != "accumulation" || m.Reason != "cvd_absorption" {
		t.Fatalf("unexpected labels: source=%q regime=%q reason=%q", m.Source, m.Regime, m.Reason)
	}

	// directional = (0.8-0.6)/(1-0.6) = 0.5; suppression = 1 (drift <= 0).
	if !approx(m.Confidence, 0.5) {
		t.Fatalf("expected confidence 0.5, got %v", m.Confidence)
	}

	// Forward horizon must be stamped (30 min) so settlement uses it.
	if m.Timeframe.End-m.Timeframe.Start != int64(cvdForwardHorizon.Seconds()) {
		t.Fatalf("expected %ds forward horizon, got %ds",
			int64(cvdForwardHorizon.Seconds()), m.Timeframe.End-m.Timeframe.Start)
	}
}

func TestCVDDivergenceWhenPriceDriftsUpWithinBand(t *testing.T) {
	state := NewCVDSymbol(testPair())
	state.FeedTrade(100, 1, true, testBase) // start price 100
	feed(state, 44, 100, true)
	feed(state, 4, 100, false)
	state.FeedTrade(100.2, 1, false, testBase) // last 100.2 => drift 0.002 (within 0.003 band)

	m, ok := state.Measure(testBase.Add(time.Minute))

	if !ok {
		t.Fatal("expected a divergence measurement")
	}

	if m.Reason != "cvd_divergence" {
		t.Fatalf("expected cvd_divergence (drift>0 within band), got %q", m.Reason)
	}

	// directional 0.5, suppression = 1 - 0.002/0.003 = 1/3 => confidence ~0.1667.
	if !approx(m.Confidence, 0.5*(1-0.002/cvdPriceFlatBand)) {
		t.Fatalf("unexpected confidence %v", m.Confidence)
	}
}

func TestCVDDistributionOnFlatPriceNetSelling(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, 45, 100, false) // net selling
	feed(state, 5, 100, true)

	m, ok := state.Measure(testBase.Add(time.Minute))

	if !ok {
		t.Fatal("expected a distribution measurement")
	}

	if m.Type != engine.Dump {
		t.Fatalf("expected Dump type, got %v", m.Type)
	}

	if m.Regime != "distribution" || m.Reason != "cvd_distribution" {
		t.Fatalf("unexpected labels: regime=%q reason=%q", m.Regime, m.Reason)
	}

	if !approx(m.Confidence, 0.5) {
		t.Fatalf("expected confidence 0.5, got %v", m.Confidence)
	}
}

func TestCVDSilentWhenFlowIsBalanced(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, 25, 100, true)
	feed(state, 25, 100, false)

	if _, ok := state.Measure(testBase.Add(time.Minute)); ok {
		t.Fatal("balanced flow (netFraction below threshold) must be silent")
	}
}

func TestCVDSilentWhenPriceRanAway(t *testing.T) {
	state := NewCVDSymbol(testPair())
	state.FeedTrade(100, 1, true, testBase)
	feed(state, 44, 100, true)
	feed(state, 5, 110, false) // last 110 => drift 0.10, well past the flat band

	// Strong net buying but price already ran 10% — not absorption, not
	// distribution: this is the "price moved with flow" case, which must not
	// produce a (late) accumulation entry.
	if _, ok := state.Measure(testBase.Add(time.Minute)); ok {
		t.Fatal("net buying with a large up-drift must not signal accumulation")
	}
}

func TestCVDWindowTrimDropsStaleTrades(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, 50, 100, true)

	// Measure 20 minutes later: every trade is older than the 15-minute window
	// and is trimmed, leaving fewer than cvdMinTrades.
	if _, ok := state.Measure(testBase.Add(20 * time.Minute)); ok {
		t.Fatal("stale trades outside the window must be trimmed, yielding no signal")
	}
}

func TestCVDIgnoresInvalidTrades(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, 50, 0, true)                // price <= 0 ignored
	state.FeedTrade(100, 0, true, testBase) // volume <= 0 ignored

	if _, ok := state.Measure(testBase.Add(time.Minute)); ok {
		t.Fatal("invalid trades must be ignored, leaving no samples")
	}
}

func TestCVDFeedQuoteUpdatesMeasurement(t *testing.T) {
	state := NewCVDSymbol(testPair())
	feed(state, 45, 100, true)
	feed(state, 5, 100, false)
	state.FeedQuote(99.5, 100.5, 100)

	measurement, ok := state.Measure(testBase.Add(time.Minute))

	if !ok {
		t.Fatal("expected measurement")
	}

	if measurement.Bid != 99.5 || measurement.Ask != 100.5 || measurement.Last != 100 {
		t.Fatalf("expected quote fields on measurement, got bid=%v ask=%v last=%v",
			measurement.Bid, measurement.Ask, measurement.Last)
	}
}

func BenchmarkCVDFeedTrade(b *testing.B) {
	state := NewCVDSymbol(testPair())

	for b.Loop() {
		state.FeedTrade(100, 1, true, testBase)
	}
}

func BenchmarkCVDMeasure(b *testing.B) {
	state := NewCVDSymbol(testPair())
	feed(state, 45, 100, true)
	feed(state, 5, 100, false)
	at := testBase.Add(time.Minute)

	for b.Loop() {
		_, _ = state.Measure(at)
	}
}
