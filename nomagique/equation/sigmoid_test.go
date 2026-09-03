package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestSigmoid(t *testing.T) {
	Convey("Given a Sigmoid equation node", t, func() {
		node := Sigmoid{}

		Convey("Zero maps to 0.5", func() {
			out := node.Step(types.Scalar(0.0))
			So(float64(out), ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("Positive large value asymptotically approaches 1.0", func() {
			out := node.Step(types.Scalar(10.0))
			expected := 1.0 / (1.0 + math.Exp(-10.0))
			So(float64(out), ShouldAlmostEqual, expected, 1e-9)
		})

		Convey("Negative large value asymptotically approaches 0.0", func() {
			out := node.Step(types.Scalar(-10.0))
			expected := 1.0 / (1.0 + math.Exp(10.0))
			So(float64(out), ShouldAlmostEqual, expected, 1e-9)
		})
	})
}
