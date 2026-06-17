package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestToxicityDynamics(testingTB *testing.T) {
	Convey("Given a symbol with tick-sized proximity", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{TickSize: "0.1"})
		state.mid = 100

		Convey("It should scale proximity to tick size", func() {
			So(state.touchProximityPct(), ShouldAlmostEqual, 0.001, 1e-12)
		})
	})

	Convey("Given churn ratio history", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{})

		for _, ratio := range []float64{0.7, 0.8, 0.9, 0.95} {
			state.gates.ChurnRatios.Observe(ratio)
		}

		Convey("It should gate churn at the 75th percentile", func() {
			So(state.gates.ChurnRatioGate(), ShouldAlmostEqual, 0.9, 1e-9)
		})
	})

	Convey("Given trade cadence", testingTB, func() {
		now := time.Now()
		state := newSymbolState(krakenmarket.Pair{})
		state.trades = []tradePrint{
			{at: now},
			{at: now.Add(time.Second)},
			{at: now.Add(2 * time.Second)},
		}

		for range 3 {
			state.timing.TradeIntervals.Observe(1)
		}

		Convey("It should derive flow alpha from cadence", func() {
			alpha := state.timing.FlowSmoothingAlpha(
				state.timing.MatchWindow(state.tradeSpan()),
				state.tradeSpan(),
				len(state.trades),
			)

			So(alpha, ShouldBeGreaterThan, 0)
			So(alpha, ShouldBeLessThan, 1)
		})
	})

	Convey("Given high-volume and low-volume trade histories", testingTB, func() {
		fastNow := time.Now()
		fast := newSymbolState(krakenmarket.Pair{})
		fast.trades = []tradePrint{
			{at: fastNow},
			{at: fastNow.Add(50 * time.Millisecond)},
			{at: fastNow.Add(100 * time.Millisecond)},
		}

		slow := newSymbolState(krakenmarket.Pair{})
		slow.trades = []tradePrint{
			{at: fastNow},
			{at: fastNow.Add(30 * time.Second)},
			{at: fastNow.Add(60 * time.Second)},
		}

		Convey("It should derive shorter match windows for fast markets", func() {
			fastWindow := fast.timing.MatchWindow(fast.tradeSpan())
			slowWindow := slow.timing.MatchWindow(slow.tradeSpan())

			So(fastWindow, ShouldBeLessThan, slowWindow)
		})
	})

	Convey("Given level lifetime history", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{})

		for _, lifetime := range []float64{0.5, 1.0, 1.5, 2.0} {
			state.timing.LevelLifetimes.Observe(lifetime)
		}

		Convey("It should derive toxic max age from observed lifetimes", func() {
			maxAge := state.timing.MaxAge()

			So(maxAge, ShouldBeGreaterThan, 0)
			So(maxAge, ShouldBeLessThan, 3*time.Second)
		})
	})

	Convey("Given book pulse history", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{})

		for _, interval := range []float64{0.01, 0.02, 0.03, 0.04} {
			state.timing.BookPulseIntervals.Observe(interval)
		}

		Convey("It should derive flash churn windows from book cadence", func() {
			window := state.timing.FlashWindow()

			So(window, ShouldBeGreaterThan, 0)
			So(window, ShouldBeLessThan, 100*time.Millisecond)
		})
	})
}

func BenchmarkToxicityDynamics(b *testing.B) {
	state := newSymbolState(krakenmarket.Pair{TickSize: "0.01"})
	state.mid = 50_000

	for _, frac := range []float64{0.05, 0.08, 0.1, 0.12, 0.15} {
		state.gates.LevelSizeFracs.Observe(frac)
	}

	for _, ratio := range []float64{0.7, 0.8, 0.85, 0.9} {
		state.gates.ChurnRatios.Observe(ratio)
	}

	for _, interval := range []float64{0.1, 0.2, 0.15, 0.18} {
		state.timing.TradeIntervals.Observe(interval)
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
		_ = state.gates.LargeBlockQtyThreshold(100, medianLevelQty(state.levels))
		_ = state.gates.ChurnRatioGate()
		_ = state.timing.MatchWindow(state.tradeSpan())
		_ = state.timing.MaxAge()
	}
}
