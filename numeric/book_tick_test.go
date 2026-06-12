package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInferBookTickSize(t *testing.T) {
	Convey("Given adjacent book prices", t, func() {
		bids := []float64{99.9, 99.8, 99.7}
		asks := []float64{100.0, 100.1, 100.2}

		Convey("It should infer the minimum positive spacing", func() {
			So(InferBookTickSize(bids, asks), ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}

func TestResolveBookTickSize(t *testing.T) {
	Convey("Given a single-level book", t, func() {
		bids := []float64{100}
		asks := []float64{101}

		Convey("It should use the configured fallback", func() {
			tickSize, err := ResolveBookTickSize(bids, asks, 0.01)

			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.01, 1e-9)
		})

		Convey("It should error when inference and fallback are unavailable", func() {
			_, err := ResolveBookTickSize(bids, asks, 0)

			So(err, ShouldNotBeNil)
		})
	})
}
