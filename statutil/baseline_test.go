package statutil

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScaleByMedian(t *testing.T) {
	Convey("Given a sample and baseline", t, func() {
		Convey("It should scale to unity with no baseline", func() {
			So(ScaleByMedian(5, nil), ShouldEqual, 1)
		})

		Convey("It should return zero for a zero sample with no baseline", func() {
			So(ScaleByMedian(0, nil), ShouldEqual, 0)
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

	Convey("Given zero-total masses", t, func() {
		masses := []float64{0, 0, 0}

		NormalizeMasses(masses)

		Convey("It should leave them unchanged for diagnostics", func() {
			So(masses, ShouldResemble, []float64{0, 0, 0})
		})
	})

	Convey("Given non-finite masses with zero total", t, func() {
		masses := []float64{math.NaN(), -1, 0}

		NormalizeMasses(masses)

		Convey("It should sanitise invalid entries but leave zero total unchanged", func() {
			So(masses[0], ShouldEqual, 0)
			So(masses[1], ShouldEqual, 0)
			So(masses[2], ShouldEqual, 0)
		})
	})
}

func TestSampleBudgetFromCadence(t *testing.T) {
	Convey("Given no cadence", t, func() {
		Convey("It should return the minimum interval budget", func() {
			So(SampleBudgetFromCadence(0), ShouldEqual, 2)
		})
	})

	Convey("Given unit cadence", t, func() {
		Convey("It should derive a symmetric interval budget", func() {
			So(SampleBudgetFromCadence(1), ShouldEqual, 2)
		})
	})

	Convey("Given fast cadence", t, func() {
		Convey("It should widen the interval budget", func() {
			So(SampleBudgetFromCadence(0.1), ShouldEqual, 11)
		})
	})
}

func TestSampleBudgetFromStamps(t *testing.T) {
	Convey("Given evenly spaced stamps", t, func() {
		stamps := []float64{0, 10, 20, 30, 40}

		Convey("It should cover the observed span in intervals", func() {
			So(SampleBudgetFromStamps(stamps), ShouldEqual, 4)
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

	Convey("Given nanosecond-scale stamps with a 0.5s gap", t, func() {
		stamps := make([]float64, 8)

		for index := range stamps {
			stamps[index] = float64(index) * 5e8
		}

		Convey("It should not explode the window beyond the sample count", func() {
			So(WindowDepth(stamps), ShouldEqual, len(stamps))
		})
	})

	Convey("Given the same logical series at ns, ms, and s scales", t, func() {
		base := []float64{0, 1, 2, 3, 4, 5, 6, 7}

		scale := func(factor float64) []float64 {
			scaled := make([]float64, len(base))

			for index, stamp := range base {
				scaled[index] = stamp * factor
			}

			return scaled
		}

		seconds := WindowDepth(scale(1))
		milliseconds := WindowDepth(scale(1e3))
		nanoseconds := WindowDepth(scale(1e9))

		Convey("It should yield the same depth at every scale", func() {
			So(milliseconds, ShouldEqual, seconds)
			So(nanoseconds, ShouldEqual, seconds)
		})
	})
}
