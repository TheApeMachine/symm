package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredictiveCoderDirectionalPrediction(t *testing.T) {
	Convey("Given a universal PredictiveCoder configured for Directional (Up/Down) forecasting", t, func() {
		// 1. Instantiate with a caller-chosen objective (Up/Down Direction)
		coder := NewPredictiveCoder(PredictiveCoderConfig{
			InputDim:      3,
			DictionaryDim: 12, // 4x overcomplete dictionary
			LatentDim:     4,
			MaxHorizon:    3,
			Target:        DirectionalTarget(0.01), // Predict Up (+1) / Down (-1)
			Pace:          0.05,
			Learn:         true,
		})

		Convey("Feeding sequence steps trains the discriminant decision boundary", func() {
			signal := 10.0
			var out PredictiveOutput
			var err error

			for step := int64(1); step <= 15; step++ {
				// Oscillating signal: steps 1-5 up, 6-10 down, 11-15 up
				if step <= 5 || step >= 11 {
					signal += 1.0
				} else {
					signal -= 1.0
				}

				out, err = coder.Step(PredictiveInput{
					Features:     []float64{0.2 * float64(step%4), -0.1, 0.5},
					Reference:    signal,
					HasReference: true,
					Step:         step,
				})
				So(err, ShouldBeNil)
			}

			// Verify directional classification & representation
			So(out.Direction, ShouldBeIn, []float64{-1.0, 0.0, 1.0})
			So(out.Confidence, ShouldBeBetweenOrEqual, 0.0, 1.0)
			So(out.InferenceSteps, ShouldBeGreaterThan, 0)
			So(out.Surprise, ShouldBeGreaterThanOrEqualTo, 0)
			So(len(out.Readout), ShouldEqual, 12+4+3+12) // [z1 (12) + z2 (4) + e0 (3) + e1 (12)]
			So(out.ResolvedSteps, ShouldBeGreaterThan, 0)
			So(out.LastResolution, ShouldNotBeNil)
			So(out.LastResolution.Target, ShouldBeIn, []float64{-1.0, 1.0})
		})
	})
}