package nomagique

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNewNumber(t *testing.T) {
	Convey("Given primitives composed with NewNumber", t, func() {
		pipeline := NewNumber(
			calculus.NewSquare(),
			calculus.NewNegate(),
		)

		Convey("When streaming inputs through the pipeline", func() {
			in := tests.SliceToSeq([]float64{2.0, 3.0})
			// 1st item: 2.0 -> square: 4.0 -> negate: -4.0
			// 2nd item: 3.0 -> square: 9.0 -> negate: -9.0
			out := tests.CollectSeq[float64](pipeline.Next(in))

			So(out, ShouldResemble, []float64{-4.0, -9.0})
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("When composing a pipeline inside another NewNumber pipeline", func() {
			outer := NewNumber(
				pipeline,
				calculus.NewAbsolute(),
			)

			in := tests.SliceToSeq([]float64{2.0})
			// 2.0 -> pipeline: -4.0 -> abs: 4.0
			out := tests.CollectSeq[float64](outer.Next(in))

			So(out, ShouldResemble, []float64{4.0})
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

		Convey("When composing heterogeneous stages (calculus to logic)", func() {
			hetero := NewNumber(
				calculus.NewSquare(),
				logic.NewFinite(),
			)
			in := tests.SliceToSeq([]float64{10.0, 20.0})
			out := tests.CollectSeq[bool](hetero.Next(in))
			So(out, ShouldResemble, []bool{true, true})
			So(hetero.Error(), ShouldBeNil)
		})
	})
}
