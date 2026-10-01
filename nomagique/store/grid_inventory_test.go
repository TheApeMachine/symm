package store_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Probe showed ~48 grid cells with venue ticker missing bid_qty/ask_qty — liquidity
never WriteMetric'd. This guards the inventory path: touch sizes must land so
signal metrics can register on the impulse map (TRAINING.md).
*/
func TestGridRegistersLiquidityWhenTouchSizePresent(t *testing.T) {
	grid := store.NewGrid()

	thin := data.NewMeasurement[float64]("websocket", nil)
	thin.Label = "BTC/USD"
	thin.WriteMetric("bid", 100)
	thin.WriteMetric("ask", 101)
	thin.WriteMetric("last", 100.5)
	thin.WriteMetric("volume", 10)
	grid.Update(thin)

	before := len(grid.Metrics)

	rich := data.NewMeasurement[float64]("liquidity:ticker", nil)
	rich.Label = "BTC/USD"
	rich.WriteMetric("bid", 100)
	rich.WriteMetric("ask", 101)
	rich.WriteMetric("bid_qty", 3)
	rich.WriteMetric("ask_qty", 4)
	rich.WriteMetric("touch_notional_imbalance", 0.1)
	rich.WriteMetric("relative_spread", 0.01)
	rich.WriteMetric("midpoint", 100.5)
	rich.WriteMetric("spread", 1)
	rich.WriteMetric("best_bid_price", 100)
	rich.WriteMetric("best_ask_price", 101)
	grid.Update(rich)

	after := len(grid.Metrics)
	if after <= before {
		t.Fatalf("expected liquidity metrics to grow inventory, before=%d after=%d", before, after)
	}
	if after-before < 5 {
		t.Fatalf("expected several new cells from liquidity WriteMetrics, grew by %d", after-before)
	}
}

func TestGridFoldsPeerMetricsIntoInventory(t *testing.T) {
	grid := store.NewGrid()

	parent := data.NewMeasurement[float64]("websocket", nil)
	parent.Label = "BTC/USD"
	parent.WriteMetric("bid", 100)
	parent.WriteMetric("ask", 101)

	peer := data.NewMeasurement[float64]("hawkes:trade", nil)
	peer.Label = "BTC/USD"
	peer.WriteMetric("excitation_fraction:buy", 0.4)
	peer.WriteMetric("excitation_fraction:sell", 0.3)
	peer.WriteMetric("conditional_intensity", 1.2)
	parent.Peers = []*data.Measurement[float64]{peer}

	grid.Update(parent)

	if len(grid.Metrics) < 5 {
		t.Fatalf("expected parent+peer metrics on grid, got %d", len(grid.Metrics))
	}
}
