package cvd

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

// This test exercises the real Signal.Step boundary, not just Sum. It belongs
// in the full SYMM checkout; the offline subset harness cannot compile the
// application's external dependencies and does not claim to have run it.
func TestSignalStepCumulativeStateAndEpochIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := NewSignal(ctx)
	signal.Transition(runtime.READY)

	observations := []struct {
		symbol             string
		epoch              int64
		at                 int64
		side               string
		price, quantity    float64
		cumulativeQuantity float64
		cumulativeNotional float64
		origin             int64
	}{
		{"BTC/USD", 1, 100, "buy", 100, 10, 10, 1000, 100},
		{"ETH/USD", 1, 110, "buy", 50, 3, 3, 150, 110},
		{"BTC/USD", 1, 120, "sell", 110, 3, 7, 670, 100},
		{"BTC/USD", 2, 130, "buy", 200, 2, 2, 400, 130},
		{"BTC/USD", 1, 140, "buy", 90, 5, 12, 1120, 100},
		{"ETH/USD", 1, 150, "sell", 75, 2, 1, 0, 110},
		{"BTC/USD", 1, 160, "sell", 80, 14, -2, 0, 100},
	}

	for index, observation := range observations {
		prior := data.NewMeasurement(
			observation.epoch, observation.symbol, "spot:trade",
			int64(index+1), int64(index+1),
			&data.StringEntry{Key: "side", Value: observation.side},
		)
		prior.At = time.Unix(observation.at, 0).UTC()
		prior.From = prior.At
		prior.Write(
			data.NewMetric("price", observation.price, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", observation.quantity, data.UnitQuantity, data.TimescaleInstantaneous),
		)

		result := signal.Step(prior)

		if result == nil {
			t.Fatalf("observation %d: nil signal output: %v", index, signal.Error())
		}

		for label, want := range map[string]float64{
			"cumulative_volume_delta":   observation.cumulativeQuantity,
			"cumulative_notional_delta": observation.cumulativeNotional,
			"cvd_epoch_from":            float64(time.Unix(observation.origin, 0).UnixNano()),
		} {
			count := 0

			for entry := range result.Read(label) {
				if entry == nil || entry.Err != nil || entry.Metric == nil {
					t.Fatalf("observation %d: invalid metric entry for %s", index, label)
				}

				count++

				if entry.Metric.Raw != want {
					t.Fatalf("observation %d %s: got %g, want %g", index, label, entry.Metric.Raw, want)
				}
			}

			if count != 1 {
				t.Fatalf("observation %d: got %d entries for %s, want 1", index, count, label)
			}
		}
	}
}
