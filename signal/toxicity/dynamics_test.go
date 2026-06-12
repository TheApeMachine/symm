package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestToxicityDynamics(t *testing.T) {
	Convey("Given a symbol with tick-sized proximity", t, func() {
		state := &symbolState{
			pair: krakenmarket.Pair{TickSize: "0.1"},
			mid:  100,
		}

		Convey("It should scale proximity to tick size", func() {
			So(state.touchProximityPct(), ShouldAlmostEqual, 0.001, 1e-12)
		})
	})

	Convey("Given churn ratio history", t, func() {
		state := &symbolState{
			churnRatios: []float64{0.7, 0.8, 0.9, 0.95},
		}

		Convey("It should gate churn at the 75th percentile", func() {
			So(state.churnRatioGate(), ShouldAlmostEqual, 0.9, 1e-9)
		})
	})

	Convey("Given trade cadence", t, func() {
		now := time.Now()
		state := &symbolState{
			trades: []tradePrint{
				{at: now},
				{at: now.Add(time.Second)},
				{at: now.Add(2 * time.Second)},
			},
		}

		Convey("It should derive flow alpha from cadence", func() {
			alpha := state.flowSmoothingAlpha(now.Add(2 * time.Second))

			So(alpha, ShouldBeGreaterThan, 0.01)
			So(alpha, ShouldBeLessThanOrEqualTo, 0.5)
		})
	})
}

func BenchmarkToxicityDynamics(b *testing.B) {
	state := &symbolState{
		pair: krakenmarket.Pair{TickSize: "0.01"},
		mid:  50_000,
		levelSizeFracs: []float64{
			0.05, 0.08, 0.1, 0.12, 0.15,
		},
		churnRatios: []float64{0.7, 0.8, 0.85, 0.9},
	}

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = state.touchProximityPct()
		_ = state.largeBlockQtyThreshold(100)
		_ = state.churnRatioGate()
	}
}
