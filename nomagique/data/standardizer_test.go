package data

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStandardizerStep(t *testing.T) {
	Convey("Given a fresh standardizer", t, func() {
		state := &standardizer{}

		Convey("the scale stays undefined until two prior observations differ", func() {
			_, scale := state.step(4)
			So(scale, ShouldEqual, 0)

			_, scale = state.step(4)
			So(scale, ShouldEqual, 0)

			_, scale = state.step(4)
			So(scale, ShouldEqual, 0)

			center, scale := state.step(10)
			So(center, ShouldEqual, 4)
			So(scale, ShouldEqual, 0)

			center, scale = state.step(4)
			So(center, ShouldAlmostEqual, 5.5, 1e-12)
			So(scale, ShouldAlmostEqual, 3, 1e-12)
		})

		Convey("each answer is the sample moments of the earlier values only", func() {
			values := []float64{3, -1, 7, 2, 2, 11, -4}

			for idx, value := range values {
				center, scale := state.step(value)

				if idx < 2 {
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
