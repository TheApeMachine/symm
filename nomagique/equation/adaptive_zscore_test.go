package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAdaptiveZScore(t *testing.T) {
	Convey("Given an AdaptiveZScore equation", t, func() {
		eq := &AdaptiveZScore{}

		Convey("On the opening sample, it seeds the baseline and reports zero divergence", func() {
			z := eq.Step(types.Scalar(0.02))

			So(z, ShouldEqual, 0.0)
			So(eq.HasPrior(), ShouldBeFalse)
			So(float64(eq.Baseline()), ShouldEqual, 0.02)
			So(float64(eq.Ratio()), ShouldEqual, 1.0)
			So(float64(eq.Divergence()), ShouldEqual, 0.0)
			So(float64(eq.Maturity()), ShouldEqual, 0.0)
		})

		Convey("On subsequent samples, it derives divergence and z-score causally from prior moments", func() {
			eq.Step(types.Scalar(0.02)) // log(0.02)
			z := eq.Step(types.Scalar(0.01))

			So(eq.HasPrior(), ShouldBeTrue)
			So(float64(eq.Baseline()), ShouldAlmostEqual, 0.02, 1e-9)
			So(float64(eq.Ratio()), ShouldAlmostEqual, 0.5, 1e-9)
			So(float64(eq.Divergence()), ShouldAlmostEqual, math.Log(0.5), 1e-9)
			So(float64(z), ShouldAlmostEqual, -1.0, 1e-9)
			So(float64(eq.Maturity()), ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}
