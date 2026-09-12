package transport_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSpreadSquareAdd(t *testing.T) {
	Convey("Nested Next is the pipeline: spread, square, add", t, func() {
		add := arithmetic.NewAdd(0.0)
		slice := []float64{4, 7, 9, 8}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&slice))
		}
		out := tests.CollectSeq[float64](add.Next(
			calculus.NewSquare().Next(
				transport.NewSpread[float64]().Next(in),
			),
		))

		So(out[len(out)-1], ShouldEqual, 210)
	})
}

func TestWindowPairs(t *testing.T) {
	Convey("Window of width 2 stride 1 yields consecutive pairs", t, func() {
		out := tests.CollectSeq[[]float64](transport.NewWindow[float64](2, 1).Next(
			transport.NewValues(1.0, 2.0, 3.0, 4.0).Next(nil),
		))

		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldResemble, []float64{1, 2})
		So(out[1], ShouldResemble, []float64{2, 3})
		So(out[2], ShouldResemble, []float64{3, 4})
	})
}

func TestApplyBindsARun(t *testing.T) {
	Convey("Apply asks the target with the bound run, not with what arrives", t, func() {
		slice := []float64{1, 2, 3}
		bound := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&slice))
		}
		dummy := []float64{9, 9}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&dummy))
		}
		spread := transport.NewApply(transport.NewSpread[float64](), bound)
		out := tests.CollectSeq[float64](spread.Next(in))

		So(out, ShouldResemble, []float64{1, 2, 3})
	})
}
