package engine

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestPredictionCalibrator(t *testing.T) {
	convey.Convey("Given no feedback", t, func() {
		calibrator := NewPredictionCalibrator()

		convey.Convey("It should stay neutral", func() {
			convey.So(calibrator.Scale(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given overconfident feedback", t, func() {
		calibrator := NewPredictionCalibrator()
		calibrator.Apply(PredictionFeedback{
			PredictedReturn: 0.1,
			ActualReturn:    0.05,
		})

		convey.Convey("It should scale parameters down", func() {
			convey.So(calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}

func TestNormalizeConfidence(t *testing.T) {
	history := []float64{0.2, 0.4, 0.6, 0.8}

	convey.Convey("Given raw confidence below the fence", t, func() {
		convey.Convey("It should normalize into unit scale", func() {
			normalized := NormalizeConfidence(0.4, history)

			convey.So(normalized, convey.ShouldBeGreaterThan, 0)
			convey.So(normalized, convey.ShouldBeLessThan, 1)
		})
	})
}

func BenchmarkPredictionCalibratorApply(b *testing.B) {
	calibrator := NewPredictionCalibrator()
	feedback := PredictionFeedback{
		PredictedReturn: 0.1,
		ActualReturn:    0.08,
	}

	b.ReportAllocs()

	for b.Loop() {
		calibrator.Apply(feedback)
	}
}
