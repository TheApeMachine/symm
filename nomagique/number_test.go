package nomagique

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
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

func TestNumberNext(t *testing.T) {
	Convey("Nested composition preserves lazy execution and error causes", t, func() {
		gather := sequence.NewGather[float64]([]int{1})
		inner := NewNumber(gather)
		outer := NewNumber(inner)
		input := sequence.NewValue([]float64{2, 5}, []float64{3})
		output := outer.Next(input)

		Convey("Constructing an iterator does not execute it", func() {
			So(outer.Error(), ShouldBeNil)
			So(gather.Error(), ShouldBeNil)
		})

		Convey("A later malformed input propagates through both compositions", func() {
			values := tests.CollectSeq[[]float64](output)
			So(values, ShouldResemble, [][]float64{{5}})
			So(errors.Is(inner.Error(), core.ErrShape), ShouldBeTrue)
			So(errors.Is(outer.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("Stopping before the malformed input does not process it", func() {
			for value := range output {
				So(*(*[]float64)(value), ShouldResemble, []float64{5})
				break
			}
			So(outer.Error(), ShouldBeNil)
		})
	})
}

func BenchmarkNumberNext(b *testing.B) {
	pipeline := NewNumber(calculus.NewSquare(), calculus.NewNegate())
	input := sequence.NewValue(1.0, 2.0, 3.0, 4.0)
	b.ReportAllocs()
	for b.Loop() {
		count := 0
		for range pipeline.Next(input) {
			count++
		}
		if count != 4 {
			b.Fatal(count)
		}
	}
}
