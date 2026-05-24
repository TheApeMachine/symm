package engine

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestCalibrationStepPenalizesLosingPredictions(t *testing.T) {
	convey.Convey("Given a positive predicted return", t, func() {
		convey.Convey("It should accept zero actual return as a losing sample", func() {
			sample, ok := CalibrationStep(0.01, 0)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(sample, convey.ShouldEqual, 0)
		})

		convey.Convey("It should accept negative actual return as a losing sample", func() {
			sample, ok := CalibrationStep(0.01, -0.005)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(sample, convey.ShouldEqual, 0)
		})

		convey.Convey("It should scale winning samples by actual/predicted", func() {
			sample, ok := CalibrationStep(0.01, 0.005)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(sample, convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}

func TestPredictionCalibratorApplyLowersScaleOnLosses(t *testing.T) {
	convey.Convey("Given an optimistic calibrator", t, func() {
		calibrator := NewPredictionCalibrator()
		calibrator.Apply(PredictionFeedback{
			PredictedReturn: 0.01,
			ActualReturn:    0.01,
		})

		convey.Convey("It should push scale toward zero on a losing forecast", func() {
			calibrator.Apply(PredictionFeedback{
				PredictedReturn: 0.01,
				ActualReturn:    -0.01,
			})

			convey.So(calibrator.Scale(), convey.ShouldBeLessThan, 1)
		})
	})
}
