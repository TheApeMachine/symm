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

func TestPredictionCalibratorApplyRejectsMissingPrediction(t *testing.T) {
	Convey("Given a settled return without a positive prediction", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		calibrator.Apply(PredictionFeedback{
			PredictedReturn: 0,
			ActualReturn:    -0.01,
		})

		Convey("It should not invent a calibration denominator", func() {
			So(calibrator.Scale(), ShouldEqual, 1)
		})
	})
}

func TestPredictionCalibratorBranchesByRegime(t *testing.T) {
	Convey("Given feedback from distinct regimes", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		calibrator.Apply(PredictionFeedback{
			Regime:          "trending",
			PredictedReturn: 0.01,
			ActualReturn:    0.01,
		})
		calibrator.Apply(PredictionFeedback{
			Regime:          "chop",
			PredictedReturn: 0.01,
			ActualReturn:    -0.01,
		})

		Convey("It should keep separate learned scales", func() {
			So(calibrator.ScaleFor("trending"), ShouldAlmostEqual, 1, 0.0001)
			So(calibrator.ScaleFor("chop"), ShouldBeLessThan, 1)
		})
	})
}

func TestCalibrationRegime(t *testing.T) {
	Convey("Given an empty feedback regime", t, func() {
		Convey("It should use an explicit default bucket", func() {
			So(CalibrationRegime(""), ShouldEqual, defaultCalibrationRegime)
			So(CalibrationRegime(" trend "), ShouldEqual, "trend")
		})
	})
}
