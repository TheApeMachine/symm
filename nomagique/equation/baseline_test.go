package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCausalBaseline(t *testing.T) {
	Convey("Given a CausalResidual equation", t, func() {
		residualEq := &CausalResidual{}

		Convey("the first observation sets baseline to itself and emits zero residual", func() {
			residual := residualEq.Step(types.Scalar(100.0))
			So(residual, ShouldEqual, 0.0)
			So(residualEq.Baseline(), ShouldEqual, 100.0)
			So(residualEq.Count(), ShouldEqual, 1.0)
		})

		Convey("subsequent observations evaluate residuals against running mean", func() {
			residualEq.Step(types.Scalar(100.0))
			residual := residualEq.Step(types.Scalar(110.0))
			So(residual, ShouldNotEqual, 0.0)
			So(residualEq.Mean(), ShouldEqual, 105.0)
			So(residualEq.Count(), ShouldEqual, 2.0)
		})
	})
}
