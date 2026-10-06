package nomagique

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNewNumber(t *testing.T) {
	Convey("Given adapter-native primitives composed with NewNumber", t, func() {
		pipeline := NewNumber(
			calculus.NewSquare(),
			calculus.NewNegate(),
		)

		run := func(composed *Number, value float64) float64 {
			adapter := data.NewAdapter(nil, data.NewState(data.NewMap("value", "value")))
			input := data.NewOutputMap()
			input.Values["value"] = value

			for range adapter.Next(data.NewValue(input)) {
			}

			So(adapter.Error(), ShouldBeNil)

			for range composed.Next(data.NewValue(adapter)) {
			}

			values := data.Read[data.Map[float64]](adapter.Next(data.NewValue(data.NewMap("value", "value"))))
			So(adapter.Error(), ShouldBeNil)

			return values.Values["value"]
		}

		Convey("When streaming an arrival through the pipeline", func() {
			So(run(pipeline, 2), ShouldEqual, -4.0)
			So(run(pipeline, 3), ShouldEqual, -9.0)
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("When composing a pipeline inside another NewNumber pipeline", func() {
			outer := NewNumber(
				pipeline,
				calculus.NewAbsolute(),
			)

			So(run(outer, 2), ShouldEqual, 4.0)
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
	})

	Convey("Given collection primitives composed with NewNumber", t, func() {
		pipeline := NewNumber(
			collection.NewOrder[float64](),
			collection.NewAt[float64](0),
		)

		Convey("It hands each arrival through every stage in order", func() {
			out := tests.CollectSeq[float64](pipeline.Next(tests.SliceToSeq([][]float64{{3, 1, 2}})))

			So(out, ShouldResemble, []float64{1})
			So(pipeline.Error(), ShouldBeNil)
		})
	})
}
