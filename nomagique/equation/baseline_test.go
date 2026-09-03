package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestCausalBaseline(t *testing.T) {
	Convey("Given a CausalBaseline equation", t, func() {
		baseline := &CausalBaseline{}

		Convey("the first observation sets baseline to itself", func() {
			residual := baseline.Step(nmtypes.Number(100.0))
			So(residual, ShouldEqual, 0.0)
			So(baseline.Baseline(), ShouldEqual, 100.0)
			So(baseline.Count(), ShouldEqual, 1.0)
		})

		Convey("subsequent observations evaluate residuals against running mean", func() {
			baseline.Step(nmtypes.Number(100.0))
			residual := baseline.Step(nmtypes.Number(110.0))
			So(residual, ShouldNotEqual, 0.0)
			So(baseline.Mean(), ShouldEqual, 105.0)
			So(baseline.Count(), ShouldEqual, 2.0)
		})
	})
}
