package matrix_test

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestProductNext(t *testing.T) {
	Convey("Given complete matrix operands", t, func() {
		Convey("Successive signed products preserve earlier results and the inputs", func() {
			node := matrix.NewProduct()
			left := [][]float64{{1, -2}, {3, 4}}
			right := [][]float64{{5, 6}, {7, 8}}

			in1 := matrix.ProductInput{Left: left, Right: right}
			seq1 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in1))
			}
			out1 := tests.CollectSeq[[][]float64](node.Next(seq1))
			So(node.Error(), ShouldBeNil)
			So(len(out1), ShouldEqual, 1)
			So(out1[0], ShouldResemble, [][]float64{{-9, -10}, {43, 50}})

			node2 := matrix.NewProduct()
			in2 := matrix.ProductInput{Left: right, Right: left}
			seq2 := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&in2))
			}
			out2 := tests.CollectSeq[[][]float64](node2.Next(seq2))
			So(node2.Error(), ShouldBeNil)
			So(len(out2), ShouldEqual, 1)
			So(out2[0], ShouldResemble, [][]float64{{23, 14}, {31, 18}})

			So(left, ShouldResemble, [][]float64{{1, -2}, {3, 4}})
			So(right, ShouldResemble, [][]float64{{5, 6}, {7, 8}})
		})

		for index, operands := range [][2][][]float64{
			{{{1}}, {{1, 2}, {3, 4}}},
			{{{1, 2}, {3}}, {{1}, {2}}},
			{{{1, 2}}, {{1, 2}, {3}}},
		} {
			Convey(fmt.Sprintf("Incompatible shape %d is reported", index), func() {
				node := matrix.NewProduct()
				input := matrix.ProductInput{Left: operands[0], Right: operands[1]}
				seq := func(yield func(unsafe.Pointer) bool) {
					yield(unsafe.Pointer(&input))
				}
				tests.CollectSeq[[][]float64](node.Next(seq))
				So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
			})
		}
	})
}
