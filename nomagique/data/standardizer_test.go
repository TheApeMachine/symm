package data

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestStandardizerStep(t *testing.T) {
	Convey("Given a fresh standardizer", t, func() {
		state := &standardizer{}

		Convey("the scale stays undefined until two prior observations differ", func() {
			for range int(core.MinimumPrior) {
				_, scale := state.step(4)
				So(scale, ShouldEqual, 0)
			}

			center, scale := state.step(10)
			So(center, ShouldEqual, 4)
			So(scale, ShouldEqual, 0)

			_, scale = state.step(4)
			So(scale, ShouldBeGreaterThan, 0)
		})

		Convey("the scale stays undefined below the minimum prior count", func() {
			// 1/sqrt(2(n-1)) <= 0.25 first holds at n = 9.
			So(core.MinimumPrior, ShouldEqual, 9)

			for idx, value := range []float64{3, -1, 7, 2, 2, 11, -4, 5, 0, 6} {
				_, scale := state.step(value)
				So(scale > 0, ShouldEqual, float64(idx) >= core.MinimumPrior)
			}
		})

		Convey("each answer is the sample moments of the earlier values only", func() {
			values := []float64{3, -1, 7, 2, 2, 11, -4, 5, 0, 6, 9, -2}

			for idx, value := range values {
				center, scale := state.step(value)

				if float64(idx) < core.MinimumPrior {
					continue
				}

				prior := values[:idx]
				mean := 0.0

				for _, earlier := range prior {
					mean += earlier
				}

				mean /= float64(len(prior))
				sumSq := 0.0

				for _, earlier := range prior {
					sumSq += (earlier - mean) * (earlier - mean)
				}

				So(center, ShouldAlmostEqual, mean, 1e-12)
				So(scale, ShouldAlmostEqual, math.Sqrt(sumSq/float64(len(prior)-1)), 1e-12)
			}
		})
	})

	Convey("Given a stream whose prior dispersion is rounding residue", t, func() {
		state := &standardizer{}

		// The stored hawkes:conditional_intensity stream for WLD/USD in epoch
		// 1791596128450467000 was a flat baseline whose third value differs by
		// one ulp, then the first excitation; the unguarded scale was 8e-18
		// and the stored z-score 4.4e17. The flat run is extended to the
		// minimum prior count so only the negligible-noise rule refuses it.
		flat := 0.070071305828507846

		for idx := range int(core.MinimumPrior) {
			value := flat

			if idx == 2 {
				value = 0.07007130582850786
			}

			state.step(value)
		}

		Convey("the z-score is undefined rather than float noise in sigma", func() {
			metric := NewMetric("conditional_intensity", 3.5965164367079705, UnitPerSecond, TimescaleInstantaneous)
			So(metric.finalize(state), ShouldBeNil)
			So(metric.Standardizable(), ShouldBeFalse)
			So(metric.Standardized, ShouldEqual, 0)
			So(metric.Normalized, ShouldEqual, 0)
		})
	})

	Convey("Given a stream in a unit that makes every value small", t, func() {
		state := &standardizer{}

		// Micro-cap prices near 1e-6 moving by 1e-9: real dispersion, far
		// below any absolute floor of one.
		for _, value := range []float64{
			1e-6, 1.001e-6, 0.999e-6, 1.002e-6, 0.998e-6,
			1e-6, 1.001e-6, 0.999e-6, 1.002e-6, 0.998e-6,
		} {
			state.step(value)
		}

		Convey("the relative guard keeps the z-score", func() {
			center, scale := state.step(1.01e-6)
			So(scale, ShouldBeGreaterThan, 0)
			So((1.01e-6-center)/scale, ShouldAlmostEqual, 6.708, 0.001)
		})
	})

	Convey("Given streams keyed by epoch, source, symbol, and metric", t, func() {
		key := standardizerKey{epoch: 7, source: "cvd", label: "BTC/USD", metric: "trade_rate"}

		Convey("the same key resolves to one stream and any differing part to another", func() {
			So(standardizerFor(key) == standardizerFor(key), ShouldBeTrue)

			other := key
			other.epoch = 8
			So(standardizerFor(other) == standardizerFor(key), ShouldBeFalse)

			other = key
			other.label = "ETH/USD"
			So(standardizerFor(other) == standardizerFor(key), ShouldBeFalse)
		})
	})
}
