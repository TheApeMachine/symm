package matrix_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDifferenceNext(t *testing.T) {
	Convey("Given two matrices", t, func() {
		node := matrix.NewDifference()
		left, right := [][]float64{{1, -2}, {3, 4}}, [][]float64{{2, 3}, {4, -5}}

		Convey("Signed subtraction preserves operands and earlier results", func() {
			first, err := transport.Evaluate(node, transport.Values(matrix.DifferenceInput{Left: left, Right: right}))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})
			second, err := transport.Evaluate(node, transport.Values(matrix.DifferenceInput{Left: right, Right: left}))
			So(err, ShouldBeNil)
			So(second, ShouldResemble, [][]float64{{1, 5}, {1, -9}})
			So(first, ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})
			So(left, ShouldResemble, [][]float64{{1, -2}, {3, 4}})
			So(right, ShouldResemble, [][]float64{{2, 3}, {4, -5}})
		})

		Convey("Unequal row counts fail", func() {
			_, err := transport.Evaluate(node, transport.Values(matrix.DifferenceInput{Left: left, Right: right[:1]}))
			So(errors.Is(err, core.ErrShape), ShouldBeTrue)
		})

		Convey("Unequal row widths fail", func() {
			_, err := transport.Evaluate(node, transport.Values(matrix.DifferenceInput{Left: left, Right: [][]float64{{1}, {2}}}))
			So(errors.Is(err, core.ErrShape), ShouldBeTrue)
		})
	})
}
