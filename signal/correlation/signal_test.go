package correlation

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

/*
TestSignal_MeasureFromMarket proves correlation on the mock Conn Session path:
sector-lift must emit herd/strength peaks with herd dominating noise on that
tape, and unstructured noise must emit NoiseScore — absolute claims, not smoke.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	symbol := conditions.Subject()
	options := tests.SessionOptions{Signals: sessionSignals}

	t.Run("tape_sector_lift", func(t *testing.T) {
		herd := tests.PlayMarketClaims(t, options, conditions.TapeSectorLift().Frames(),
			tests.SourceClaim{
				Source: types.SourceCorrelation, Metric: types.MetricHerdScore,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceCorrelation, Metric: types.MetricStrength,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)

		herdScore, hasHerd := tests.PeakSourceMetric(
			herd, types.SourceCorrelation, symbol, types.MetricHerdScore,
		)
		noiseOnHerd, hasNoise := tests.PeakSourceMetric(
			herd, types.SourceCorrelation, symbol, types.MetricNoiseScore,
		)

		if !hasHerd {
			t.Fatal("want HerdScore on sector-lift tape")
		}

		if !hasNoise {
			t.Fatal("want NoiseScore on sector-lift tape")
		}

		if herdScore <= noiseOnHerd {
			t.Fatalf(
				"want HerdScore (%g) > NoiseScore (%g) on sector-lift",
				herdScore, noiseOnHerd,
			)
		}
	})

	t.Run("tape_noise", func(t *testing.T) {
		tests.PlayMarketClaims(t, options, conditions.TapeNoise().Frames(),
			tests.SourceClaim{
				Source: types.SourceCorrelation, Metric: types.MetricNoiseScore,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session, err := tests.NewSession(context.Background(), benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	frames := conditions.TapeSectorLift().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
}
