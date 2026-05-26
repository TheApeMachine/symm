package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolConfidenceMeasure(t *testing.T) {
	Convey("Given a cold symbol confidence tracker", t, func() {
		tracker := NewSymbolConfidence(DefaultCalibrationParams())

		Convey("It should reject the first raw scores until history warms", func() {
			_, ok := tracker.Measure(0.8)

			So(ok, ShouldBeFalse)
		})
	})

	Convey("Given a warmed symbol confidence tracker", t, func() {
		tracker := NewSymbolConfidence(DefaultCalibrationParams())
		WarmSymbolConfidence(tracker, 0.2, 0.3, 0.4, 0.5)

		Convey("It should emit normalized confidence strictly below certainty", func() {
			confidence, ok := tracker.Measure(0.35)

			So(ok, ShouldBeTrue)
			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}

func TestNormalizeConfidenceNeverCertain(t *testing.T) {
	Convey("Given raw strength at or above the historical fence", t, func() {
		calibrator := NewPredictionCalibrator(DefaultCalibrationParams())
		history := []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}
		fence := ConfidenceFence(history)

		Convey("It should saturate below one", func() {
			atFence := calibrator.NormalizeConfidence(fence, history)
			aboveFence := calibrator.NormalizeConfidence(fence*10, history)

			So(atFence, ShouldEqual, 0.5)
			So(aboveFence, ShouldBeGreaterThan, atFence)
			So(aboveFence, ShouldBeLessThan, 1)
		})
	})
}

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
