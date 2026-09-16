package arithmetic_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMatrixDifferenceNext(t *testing.T) {
	Convey("Given two matrices", t, func() {
		left, right := [][]float64{{1, -2}, {3, 4}}, [][]float64{{2, 3}, {4, -5}}

		Convey("Signed subtraction preserves operands and earlier results", func() {
			node1 := arithmetic.NewMatrixDifference()
			in1 := arithmetic.DifferenceInput{Left: left, Right: right}
			seq1 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in1))
			}
			first := tests.CollectSeq[[][]float64](node1.Next(seq1))
			So(node1.Error(), ShouldBeNil)
			So(first[0], ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})

			node2 := arithmetic.NewMatrixDifference()
			in2 := arithmetic.DifferenceInput{Left: right, Right: left}
			seq2 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in2))
			}
			second := tests.CollectSeq[[][]float64](node2.Next(seq2))
			So(node2.Error(), ShouldBeNil)
			So(second[0], ShouldResemble, [][]float64{{1, 5}, {1, -9}})

			So(first[0], ShouldResemble, [][]float64{{-1, -5}, {-1, 9}})
			So(left, ShouldResemble, [][]float64{{1, -2}, {3, 4}})
			So(right, ShouldResemble, [][]float64{{2, 3}, {4, -5}})
		})

		Convey("Unequal row counts fail", func() {
			node := arithmetic.NewMatrixDifference()
			in := arithmetic.DifferenceInput{Left: left, Right: right[:1]}
			seq := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in))
			}
			tests.CollectSeq[[][]float64](node.Next(seq))
			So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("Unequal row widths fail", func() {
			node := arithmetic.NewMatrixDifference()
			in := arithmetic.DifferenceInput{Left: left, Right: [][]float64{{1}, {2}}}
			seq := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in))
			}
			tests.CollectSeq[[][]float64](node.Next(seq))
			So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
