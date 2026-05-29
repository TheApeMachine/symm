package depthflow

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestDepthSymbolRejectsSpoofSkew(t *testing.T) {
	state := NewDepthSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	state.bids = []market.BookLevel{
		{Price: 100, Volume: 1},
		{Price: 99.5, Volume: 100},
	}
	state.asks = []market.BookLevel{
		{Price: 100.1, Volume: 10},
	}
	state.buyPressure = 1

	if measurement, ok := state.Measure(); !ok {
		t.Fatal("expected spoof skew to emit skeptical confidence")
	} else if measurement.Reason != "depth_skeptic" || measurement.Confidence <= 0 {
		t.Fatalf("unexpected spoof measurement: %+v", measurement)
	}
}

func TestDepthSymbolMeasureTradePressure(t *testing.T) {
	state := NewDepthSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.last = 10
	state.bid = 9.99
	state.ask = 10.01
	state.buyPressure = 0.8

	measurement, ok := state.Measure()

	if !ok {
		t.Fatal("expected trade pressure measurement")
	}

	if measurement.Reason != "trade_pressure" || measurement.Confidence <= 0 {
		t.Fatalf("unexpected trade pressure measurement: %+v", measurement)
	}
}

func TestDepthSymbolAcceptsAlignedTouch(t *testing.T) {
	state := NewDepthSymbol(asset.Pair{Wsname: "PUMP/EUR"})
	state.bids = []market.BookLevel{
		{Price: 100, Volume: 80},
	}
	state.asks = []market.BookLevel{
		{Price: 100.1, Volume: 20},
	}
	state.buyPressure = 1

	for range 8 {
		_, _ = state.score.Push(0.8, 1)
	}

	if _, ok := state.Measure(); !ok {
		t.Fatal("expected aligned touch imbalance to pass")
	}
}
