package matrix_test

import (
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestDifferenceNext(t *testing.T) {
	Convey("Given two matrix expressions", t, func() {
		node := matrix.NewDifference(store.NewGet("left"), store.NewGet("right"))
		left, right := [][]float64{{1, -2}, {3, 4}}, [][]float64{{2, 3}, {4, -5}}

		Convey("Signed subtraction preserves operands and earlier results", func() {
			first, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": left, "right": right}))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})
			second, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": right, "right": left}))
			So(err, ShouldBeNil)
			So(second, ShouldResemble, [][]float64{{1, 5}, {1, -9}})
			So(first, ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})
			So(left, ShouldResemble, [][]float64{{1, -2}, {3, 4}})
			So(right, ShouldResemble, [][]float64{{2, 3}, {4, -5}})
		})

		Convey("Unequal row counts fail", func() {
			_, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": left, "right": right[:1]}))
			So(errors.Is(err, core.ErrShape), ShouldBeTrue)
		})

		Convey("Unequal row widths fail", func() {
			_, err := transport.Evaluate[[][]float64](node, tests.Record(map[string]any{"left": left, "right": [][]float64{{1}, {2}}}))
			So(errors.Is(err, core.ErrShape), ShouldBeTrue)
		})
	})
}
