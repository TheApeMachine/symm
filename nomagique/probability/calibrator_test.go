package probability

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCalibrator(t *testing.T) {
	Convey("Given an empirical error calibrator", t, func() {
		calibrator := NewCalibrator(CalibratorConfig{
			Window: 4,
		})

		Convey("Then the first reading scores zero because no prior history exists", func() {
			out, err := calibrator.Measure(10)
			So(err, ShouldBeNil)
			So(out.Value, ShouldEqual, 0)
			So(out.Ready, ShouldBeFalse)
		})

		Convey("Then subsequent readings are scored against prior history", func() {
			calibrator.Quantile(10) // history: [10]
			calibrator.Quantile(20) // history: [10, 20]
			calibrator.Quantile(30) // history: [10, 20, 30]

			// reading of 15 beats 20 and 30 out of 3 prior samples -> 2/3
			out, err := calibrator.Measure(15)
			So(err, ShouldBeNil)
			So(out.Value, ShouldAlmostEqual, 2.0/3.0, 1e-6)
		})

		Convey("Then the ring buffer wraps around without exceeding capacity", func() {
			calibrator.Quantile(10)
			calibrator.Quantile(20)
			calibrator.Quantile(30)
			calibrator.Quantile(40)
			calibrator.Quantile(50) // wraps: replaces 10

			So(calibrator.Count(), ShouldEqual, 4)
		})

		Convey("Then non-finite values return validation errors", func() {
			_, err := calibrator.Measure(math.NaN())
			So(err, ShouldNotBeNil)

			_, errInf := calibrator.Measure(math.Inf(1))
			So(errInf, ShouldNotBeNil)
		})

		Convey("Then reset clears all state", func() {
			calibrator.Quantile(10)
			calibrator.Quantile(20)
			calibrator.Reset()

			So(calibrator.Count(), ShouldEqual, 0)
			out, err := calibrator.Measure(10)
			So(err, ShouldBeNil)
			So(out.Value, ShouldEqual, 0)
			So(out.Ready, ShouldBeFalse)
		})
	})
}

func BenchmarkCalibrator(b *testing.B) {
	calibrator := NewCalibrator()

	for b.Loop() {
		_ = calibrator.Quantile(1.234)
	}
}
