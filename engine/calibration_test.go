package engine

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCalibrationStepPenalizesLosingPredictions(t *testing.T) {
	Convey("Given a positive predicted return", t, func() {
		Convey("It should accept zero actual return as a losing sample", func() {
			sample, ok := CalibrationStep(0.01, 0)

			So(ok, ShouldBeTrue)
			So(sample, ShouldEqual, 1)
		})

		Convey("It should accept negative actual return as a losing sample", func() {
			sample, ok := CalibrationStep(0.01, -0.005)

			So(ok, ShouldBeTrue)
			So(sample, ShouldAlmostEqual, 0.5, 0.0001)
		})

		Convey("It should scale winning samples by actual/predicted", func() {
			sample, ok := CalibrationStep(0.01, 0.005)

			So(ok, ShouldBeTrue)
			So(sample, ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}

func TestAdaptiveCalibrationHalfLifeScalesWithRunway(t *testing.T) {
	Convey("Given short and long runways", t, func() {
		params := DefaultCalibrationParams()
		short := params.adaptiveHalfLife(2 * time.Second)
		long := params.adaptiveHalfLife(15 * time.Minute)

		Convey("It should keep short signals on tighter windows", func() {
			So(short, ShouldBeLessThan, long)
			So(short, ShouldBeGreaterThanOrEqualTo, params.HalfLifeFloor)
		})
	})
}

func TestPredictionCalibratorApplyLowersScaleOnLosses(t *testing.T) {
	Convey("Given an optimistic calibrator", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		calibrator.Apply(PredictionFeedback{
			PredictedReturn: 0.01,
			ActualReturn:    0.01,
		})

		Convey("It should push scale toward zero on a losing forecast", func() {
			calibrator.Apply(PredictionFeedback{
				PredictedReturn: 0.01,
				ActualReturn:    -0.01,
			})

			So(calibrator.Scale(), ShouldBeLessThan, 1)
		})
	})
}
