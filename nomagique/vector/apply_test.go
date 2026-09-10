package vector_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

func TestApplyNext(t *testing.T) {
	Convey("Apply sends each arrival to the aligned operation", t, func() {
		for _, values := range [][]float64{{4, 5}, {-2, 7}} {
			node := vector.NewApply[float64, float64](
				arithmetic.NewAdd[float64, float64](2),
				arithmetic.NewMultiply[float64](3),
			)
			out := tests.CollectSeq(node.Next(transport.Values(values...)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, values[0]+2)
			So(out[1], ShouldEqual, values[1]*3)
		}
	})
}

func TestApplyIndependentState(t *testing.T) {
	Convey("Each coordinate owns independent recurrence", t, func() {
		node := vector.NewApply(
			equation.NewWelford(),
			equation.NewWelford(),
		)

		for index, pair := range [][2]float64{{2, 20}, {4, 40}, {6, 60}} {
			out := tests.CollectSeq(node.Next(transport.Values(pair[0], pair[1])))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0].Count, ShouldEqual, float64(index+1))
			So(out[1].Count, ShouldEqual, float64(index+1))
			So(out[0].Mean, ShouldEqual, float64(index+2))
			So(out[1].Mean, ShouldEqual, 10*float64(index+2))
		}
	})
}

func TestApplyShape(t *testing.T) {
	Convey("Unequal endpoint cardinalities are a shape error", t, func() {
		node := vector.NewApply(arithmetic.NewAdd[float64, float64](0))
		tests.CollectSeq(node.Next(transport.Values(1.0, 2.0)))
		So(node.Error(), ShouldNotBeNil)
	})
}
