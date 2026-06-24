package statutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScaleByMedian(t *testing.T) {
	Convey("Given a sample and baseline", t, func() {
		Convey("It should return zero with no baseline", func() {
			So(ScaleByMedian(5, nil), ShouldEqual, 0)
		})

		Convey("It should scale by the baseline median", func() {
			So(ScaleByMedian(4, []float64{1, 2, 3}), ShouldAlmostEqual, 2, 1e-12)
		})
	})
}

func TestNormalizeMasses(t *testing.T) {
	Convey("Given raw category masses", t, func() {
		masses := []float64{1, 1, 2}

		NormalizeMasses(masses)

		Convey("It should normalise them to sum to one", func() {
			So(masses[0], ShouldAlmostEqual, 0.25, 1e-12)
			So(masses[1], ShouldAlmostEqual, 0.25, 1e-12)
			So(masses[2], ShouldAlmostEqual, 0.5, 1e-12)
		})
	})
}

func TestWindowDepth(t *testing.T) {
	Convey("Given evenly spaced stamps", t, func() {
		stamps := []float64{0, 10, 20, 30, 40}

		Convey("It should keep stamps inside the cadence window", func() {
			So(WindowDepth(stamps), ShouldEqual, 5)
		})
	})
}
