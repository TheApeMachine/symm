package vector_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/vector"
)

func TestApplyNext(t *testing.T) {
	Convey("Apply sends each arrival to the aligned operation", t, func() {
		for _, values := range [][]float64{{4, 5}, {-2, 7}} {
			orig0, orig1 := values[0], values[1]
			node := vector.NewApply(
				arithmetic.NewAdd(2),
				arithmetic.NewMultiply(3),
			)
			out := tests.CollectSeq[float64](node.Next(tests.SliceToSeq(values)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, orig0+2)
			So(out[1], ShouldEqual, orig1*3)
		}
	})
}

func TestApplyIndependentState(t *testing.T) {
	Convey("Each coordinate owns independent recurrence", t, func() {
		node := vector.NewApply(
			arithmetic.NewAdd(0),
			arithmetic.NewAdd(10),
		)

		for _, pair := range [][2]float64{{2, 20}, {4, 40}, {6, 60}} {
			out := tests.CollectSeq[float64](node.Next(tests.SliceToSeq([]float64{pair[0], pair[1]})))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
		}
	})
}

func TestApplyShape(t *testing.T) {
	Convey("Unequal endpoint cardinalities are a shape error", t, func() {
		node := vector.NewApply(arithmetic.NewAdd(0))
		tests.CollectSeq[float64](node.Next(tests.SliceToSeq([]float64{1.0, 2.0})))
		So(node.Error(), ShouldNotBeNil)
	})
}
