package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPeakNext(t *testing.T) {
	Convey("Peak selects the first absolute maximum", t, func() {
		op := equation.NewPeak()
		out := tests.CollectSeq(op.Next(transport.Values(
			equation.Point{Y: 0.1},
			equation.Point{Y: -0.9},
			equation.Point{Y: 0.9},
		)))

		So(op.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0].Index, ShouldEqual, 1)
		So(out[0].Point.Y, ShouldEqual, -0.9)

		Convey("An empty run cannot repeat the preceding winner", func() {
			So(tests.CollectSeq(op.Next(transport.Values[equation.Point]())), ShouldBeEmpty)
			So(op.Error(), ShouldBeNil)
		})
	})
}
