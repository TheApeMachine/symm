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
			tradeIntervals: []float64{1, 1, 1},
		}

		Convey("It should derive flow alpha from cadence", func() {
			alpha := state.flowSmoothingAlpha(now.Add(2 * time.Second))

			So(alpha, ShouldBeGreaterThan, 0)
			So(alpha, ShouldBeLessThan, 1)
		})
	})

	Convey("Given high-volume and low-volume trade histories", t, func() {
		fastNow := time.Now()
		fast := &symbolState{
			trades: []tradePrint{
				{at: fastNow},
				{at: fastNow.Add(50 * time.Millisecond)},
				{at: fastNow.Add(100 * time.Millisecond)},
			},
		}
		slow := &symbolState{
			trades: []tradePrint{
				{at: fastNow},
				{at: fastNow.Add(30 * time.Second)},
				{at: fastNow.Add(60 * time.Second)},
			},
		}

		Convey("It should derive shorter match windows for fast markets", func() {
			fastWindow := fast.tradeMatchWindow(fastNow.Add(100 * time.Millisecond))
			slowWindow := slow.tradeMatchWindow(fastNow.Add(60 * time.Second))

			So(fastWindow, ShouldBeLessThan, slowWindow)
		})
	})

	Convey("Given level lifetime history", t, func() {
		state := &symbolState{
			levelLifetimes: []float64{0.5, 1.0, 1.5, 2.0},
		}

		Convey("It should derive toxic max age from observed lifetimes", func() {
			maxAge := state.toxicMaxAge()

			So(maxAge, ShouldBeGreaterThan, 0)
			So(maxAge, ShouldBeLessThan, 3*time.Second)
		})
	})

	Convey("Given book pulse history", t, func() {
		state := &symbolState{
			bookPulseIntervals: []float64{0.01, 0.02, 0.03, 0.04},
		}

		Convey("It should derive flash churn windows from book cadence", func() {
			window := state.flashChurnWindow()

			So(window, ShouldBeGreaterThan, 0)
			So(window, ShouldBeLessThan, 100*time.Millisecond)
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
		tradeIntervals: []float64{
			0.1, 0.2, 0.15, 0.18,
		},
	}

	now := time.Now()
	state.trades = []tradePrint{
		{at: now},
		{at: now.Add(100 * time.Millisecond)},
		{at: now.Add(250 * time.Millisecond)},
	}

	b.ResetTimer()

	for b.Loop() {
		_ = state.touchProximityPct()
		_ = state.largeBlockQtyThreshold(100)
		_ = state.churnRatioGate()
		_ = state.tradeMatchWindow(now)
		_ = state.toxicMaxAge()
	}
}
