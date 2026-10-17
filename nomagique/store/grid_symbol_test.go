package store

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/data"
)

func TestSymbolRegionsKeepsEachSymbol(t *testing.T) {
	grid := NewGrid(context.Background())

	feed := func(btc, eth float64) {
		t.Helper()

		value := data.NewValue("kraken", "trade", "aggregate")
		data.AppendSignalVector(value, "cvd", "BTC/USD", []float64{btc}, "midpoint_log_return")
		data.AppendSignalVector(value, "cvd", "ETH/USD", []float64{eth}, "midpoint_log_return")

		if _, err := grid.Observe(value); err != nil {
			t.Fatal(err)
		}
	}

	feed(1, 1)
	feed(2, 3)
	feed(4, 8)

	got := grid.SymbolRegions()

	if _, ok := got["BTC/USD"]; !ok {
		t.Fatalf("BTC/USD missing from %v", got)
	}

	if _, ok := got["ETH/USD"]; !ok {
		t.Fatalf("ETH/USD missing from %v", got)
	}
}
