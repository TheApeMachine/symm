package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSpreadSquareAdd(t *testing.T) {
	Convey("Nested Next is the pipeline: spread, square, add", t, func() {
		add := arithmetic.NewAdd[float64, float64](0.0)
		out := tests.CollectSeq(add.Next(
			calculus.NewSquare[float64]().Next(
				NewSpread[float64]().Next(Values([]float64{4, 7, 9, 8})),
			),
		))

		So(out[len(out)-1], ShouldEqual, 210)
	})
}

func TestWindowPairs(t *testing.T) {
	Convey("Window of width 2 stride 1 yields consecutive pairs", t, func() {
		out := tests.CollectSeq(NewWindow[float64](2, 1).Next(Values(1.0, 2.0, 3.0, 4.0)))

		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldResemble, []float64{1, 2})
		So(out[1], ShouldResemble, []float64{2, 3})
		So(out[2], ShouldResemble, []float64{3, 4})
	})
}

func TestApplyBindsARun(t *testing.T) {
	Convey("Apply asks the target with the bound run, not with what arrives", t, func() {
		spread := NewApply(NewSpread[float64](), Values([]float64{1, 2, 3}))
		out := tests.CollectSeq(spread.Next(Values([]float64{9, 9})))

		So(out, ShouldResemble, []float64{1, 2, 3})
	})
}
