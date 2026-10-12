package data

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
midRankZ is the brute-force z-score of value among priors: the standard
normal quantile of its mid-rank among the priors and itself.
*/
func midRankZ(priors []float64, value float64) float64 {
	below, equal := 0.0, 0.0

	for _, prior := range priors {
		if prior < value {
			below++
		}

		if prior == value {
			equal++
		}
	}

	count := float64(len(priors))
	probability := (below + (equal+core.Unit)/2) / (count + core.Unit)

	return math.Sqrt2 * math.Erfinv(2*probability-core.Unit)
}

/*
edgeZ is the largest z-score a stream with count priors can produce: a value
beyond every prior.
*/
func edgeZ(count float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*(count+core.Unit/2)/(count+core.Unit)-core.Unit)
}

func TestStandardizerObserve(t *testing.T) {
	Convey("Given a fresh standardizer", t, func() {
		state := &standardizer{}

		Convey("the z-score stays undefined below the minimum rank prior", func() {
			// 1/sqrt(n) <= 0.25 first holds at n = 16.
			So(core.MinimumRankPrior, ShouldEqual, 16)

			for index := range int(core.MinimumRankPrior) {
				_, support, defined := state.observe(float64(index))
				So(defined, ShouldBeFalse)
				So(support, ShouldEqual, 0)
			}

			_, support, defined := state.observe(0)
			So(defined, ShouldBeTrue)
			So(support, ShouldEqual, core.MinimumRankPrior)
		})

		Convey("each answer is the mid-rank quantile among the earlier values only", func() {
			values := []float64{
				3, -1, 7, 2, 2, 11, -4, 5, 0, 6, 9, -2, 2, 2, 8, 1,
				2, 30, -7, 2, 5, 5, 0, 11, 2, -1, 4, 4, 13, 2,
			}

			for index, value := range values {
				standardized, support, defined := state.observe(value)

				if float64(index) < core.MinimumRankPrior {
					So(defined, ShouldBeFalse)

					continue
				}

				So(defined, ShouldBeTrue)
				So(support, ShouldEqual, float64(index))
				So(standardized, ShouldAlmostEqual, midRankZ(values[:index], value), 1e-12)
			}
		})
	})

	Convey("Given a stream that sat on one value", t, func() {
		state := &standardizer{}

		for range int(core.MinimumRankPrior) {
			state.observe(4)
		}

		Convey("the plateau value standardizes to exactly zero", func() {
			standardized, _, defined := state.observe(4)
			So(defined, ShouldBeTrue)
			So(standardized, ShouldEqual, 0)
		})
	})

	Convey("Given the stored BTC/USD relative_spread: a plateau, then a 30x jump", t, func() {
		state := &standardizer{}

		// The mean/sigma z-score of this jump was stored as 4.137e6.
		for range int(core.MinimumRankPrior) {
			state.observe(1.2e-5)
		}

		Convey("the jump is placed at the edge of its priors", func() {
			standardized, support, defined := state.observe(3.6e-4)
			So(defined, ShouldBeTrue)
			So(standardized, ShouldAlmostEqual, edgeZ(support), 1e-12)
		})
	})

	Convey("Given the stored WLD/USD conditional_intensity: one-ulp residue, then excitation", t, func() {
		state := &standardizer{}

		// The mean/sigma scale of this stream was 8e-18 and the stored
		// z-score 4.4e17.
		flat := 0.070071305828507846

		for index := range int(core.MinimumRankPrior) {
			value := flat

			if index == 2 {
				value = 0.07007130582850786
			}

			state.observe(value)
		}

		Convey("the excitation is placed at the edge of its priors", func() {
			metric := NewMetric("conditional_intensity", 3.5965164367079705, UnitPerSecond, TimescaleInstantaneous)
			So(metric.finalize(state), ShouldBeNil)
			So(metric.Standardizable(), ShouldBeTrue)
			So(metric.Standardized, ShouldAlmostEqual, edgeZ(core.MinimumRankPrior), 1e-12)
		})
	})

	Convey("Given a micro-cap stream whose values are all near 1e-6", t, func() {
		state := &standardizer{}
		values := []float64{
			1e-6, 1.001e-6, 0.999e-6, 1.002e-6, 0.998e-6,
			1e-6, 1.001e-6, 0.999e-6, 1.002e-6, 0.998e-6,
			1e-6, 1.001e-6, 0.999e-6, 1.002e-6, 0.998e-6,
			1e-6,
		}

		for _, value := range values {
			state.observe(value)
		}

		Convey("the scale of the unit does not matter, only the order", func() {
			standardized, _, defined := state.observe(1.0015e-6)
			So(defined, ShouldBeTrue)
			So(standardized, ShouldAlmostEqual, midRankZ(values, 1.0015e-6), 1e-12)
			So(standardized, ShouldBeGreaterThan, 0)
			So(standardized, ShouldBeLessThan, edgeZ(float64(len(values))))
		})
	})

	Convey("Given streams keyed by epoch, source, symbol, and metric", t, func() {
		key := standardizerKey{epoch: 7, source: "cvd", label: "BTC/USD", metric: "trade_rate"}

		Convey("the same key resolves to one stream and any differing part to another", func() {
			first := standardizerFor(key)
			second := standardizerFor(key)
			So(second, ShouldPointTo, first)

			differentEpoch := key
			differentEpoch.epoch = 8
			So(standardizerFor(differentEpoch), ShouldNotPointTo, first)
			So(differentEpoch.seed(), ShouldNotEqual, key.seed())

			differentLabel := key
			differentLabel.label = "ETH/USD"
			So(standardizerFor(differentLabel), ShouldNotPointTo, first)
			So(differentLabel.seed(), ShouldNotEqual, key.seed())
		})

		Convey("the seed is a pure function of the identity", func() {
			So(key.seed(), ShouldEqual, standardizerKey{epoch: 7, source: "cvd", label: "BTC/USD", metric: "trade_rate"}.seed())
		})
	})
}
