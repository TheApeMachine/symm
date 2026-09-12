package nomagique

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNewNumber(t *testing.T) {
	Convey("Given arithmetic primitives composed with NewNumber", t, func() {
		// (input + 10) * 2
		pipeline := NewNumber(
			arithmetic.NewAdd(10.0),
			arithmetic.NewMultiply(2.0),
		)

		Convey("When streaming inputs through the pipeline", func() {
			in := tests.SliceToSeq([]float64{1.0, 2.0})
			// 1st item: 1.0 -> add: 10 + 1 = 11 -> mult: 2 * 11 = 22
			// 2nd item: 2.0 -> add: 11 + 2 = 13 -> mult: 22 * 13 = 286
			out := tests.CollectSeq[float64](pipeline.Next(in))

			So(out, ShouldResemble, []float64{22.0, 286.0})
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("When composing a pipeline inside another NewNumber pipeline", func() {
			outer := NewNumber(
				pipeline,
				arithmetic.NewSubtract(100.0),
			)

			in := tests.SliceToSeq([]float64{1.0})
			// 1.0 -> pipeline -> 22.0 -> sub: 100 - 22 = 78
			out := tests.CollectSeq[float64](outer.Next(in))

			So(out, ShouldResemble, []float64{78.0})
			So(outer.Error(), ShouldBeNil)
		})

		Convey("When input is nil", func() {
			out := tests.CollectSeq[float64](pipeline.Next(nil))
			So(len(out), ShouldEqual, 0)
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("When error is recorded on pipeline or stage", func() {
			expectedErr := errors.New("pipeline test error")
			pipeline.Error(expectedErr)
			So(errors.Is(pipeline.Error(), expectedErr), ShouldBeTrue)
		})

		Convey("When composing heterogeneous stages (arithmetic to logic)", func() {
			hetero := NewNumber(
				arithmetic.NewAdd(0.0),
				logic.NewFinite(),
			)
			in := tests.SliceToSeq([]float64{10.0, 20.0})
			out := tests.CollectSeq[bool](hetero.Next(in))
			So(out, ShouldResemble, []bool{true, true})
			So(hetero.Error(), ShouldBeNil)
		})
	})
}
