package types

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhaseReadingInference(t *testing.T) {
	Convey("Given constructive and antipodal historical phase matches", t, func() {
		reading := PhaseReading{
			Ready: true,
			Responses: []PhaseResponse{
				{Similarity: 0.8, Outcome: PhaseOutcome{Direction: "up"}},
				{Similarity: -0.6, Outcome: PhaseOutcome{Direction: "down"}},
				{Similarity: 0.2, Outcome: PhaseOutcome{Direction: "down"}},
			},
		}

		inference, ready := reading.Inference()

		Convey("The antipodal down analogue should support the current up direction", func() {
			So(ready, ShouldBeTrue)
			So(inference.Support, ShouldAlmostEqual, 1.4, 1e-12)
			So(inference.Contradiction, ShouldAlmostEqual, 0.2, 1e-12)
			So(inference.Direction, ShouldEqual, float64(1))
			So(inference.Confidence, ShouldAlmostEqual, math.Abs(1.2/1.6), 1e-12)
		})
	})

	Convey("Given only flat outcome labels", t, func() {
		_, ready := (PhaseReading{
			Ready: true,
			Responses: []PhaseResponse{{
				Similarity: 1,
				Outcome:    PhaseOutcome{Direction: "flat"},
			}},
		}).Inference()
		So(ready, ShouldBeFalse)
	})
}
