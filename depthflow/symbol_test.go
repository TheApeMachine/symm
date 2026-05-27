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

	if _, ok := state.Measure(); ok {
		t.Fatal("expected spoof skew to be rejected")
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
