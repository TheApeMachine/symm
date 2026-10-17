package store

import (
	"context"
	"math"
	"testing"

	"github.com/theapemachine/symm/data"
)

func TestEnergyIsTheSumOfSquaredMoves(t *testing.T) {
	grid := NewGrid(context.Background())

	feed := func(value float64) {
		t.Helper()

		tick := data.NewValue("kraken", "trade", "aggregate")
		data.AppendSignalVector(tick, "cvd", "BTC/USD", []float64{value}, "midpoint_log_return")

		if _, err := grid.Observe(tick); err != nil {
			t.Fatal(err)
		}
	}

	feed(1)

	if grid.Energy("BTC/USD") != 0 {
		t.Fatalf("energy before a prior = %v", grid.Energy("BTC/USD"))
	}

	feed(100)

	got := grid.Energy("BTC/USD")
	want := math.Pow(SNM(100, 1), 2)

	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("energy = %v, want %v", got, want)
	}
}
