package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeConfidenceColdSymbol(t *testing.T) {
	Convey("Given no calibration history", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())

		Convey("It should return zero confidence", func() {
			So(calibrator.NormalizeConfidence(0.8, nil), ShouldEqual, 0)
		})
	})
}

func TestNormalizeConfidenceCalibratedSymbol(t *testing.T) {
	Convey("Given enough calibration history", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		history := []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}
		fence := ConfidenceFence(history)

		Convey("It should scale raw strength against the fence", func() {
			score := calibrator.NormalizeConfidence(fence/2, history)

			So(score, ShouldBeGreaterThan, 0)
			So(score, ShouldBeLessThan, 1)
		})
	})
}

func TestNormalizeConfidenceUsesMinConfidenceHistory(t *testing.T) {
	Convey("Given the minimum confidence history length", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		history := []float64{0.2, 0.4, 0.6, 0.8}

		Convey("It should normalize once history is warm", func() {
			So(calibrator.NormalizeConfidence(0.4, history), ShouldBeGreaterThan, 0)
		})
	})
}
