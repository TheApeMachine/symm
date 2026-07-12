package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCalibrationObserve(t *testing.T) {
	Convey("Given walk-forward predictions that beat the zero-return baseline", t, func() {
		calibration := &Calibration{}

		for index := 0; index < forecastFeatureCount+2; index++ {
			actual := float64(index+1) / 100
			calibration.Observe(actual, actual)
		}

		Convey("It should report positive lower-bound improvement", func() {
			snapshot := calibration.Snapshot(forecastFeatureCount)
			So(snapshot.Samples, ShouldEqual, uint64(forecastFeatureCount+2))
			So(snapshot.IncrementalMSE, ShouldBeGreaterThan, 0)
			So(snapshot.LowerBound, ShouldBeGreaterThan, 0)
			So(snapshot.Calibrated, ShouldBeTrue)
		})
	})
}

func BenchmarkCalibrationObserve(b *testing.B) {
	calibration := &Calibration{}

	for b.Loop() {
		calibration.Observe(0.01, 0.011)
	}
}
