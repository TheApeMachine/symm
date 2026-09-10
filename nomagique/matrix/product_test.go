package matrix_test

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestProductNext(t *testing.T) {
	Convey("Given complete matrix operands", t, func() {
		Convey("Successive signed products preserve earlier results and the inputs", func() {
			node := matrix.NewProduct()
			left := [][]float64{{1, -2}, {3, 4}}
			right := [][]float64{{5, 6}, {7, 8}}
			first, err := transport.Evaluate(node, transport.Values(matrix.ProductInput{Left: left, Right: right}))
			So(err, ShouldBeNil)
			So(first, ShouldResemble, [][]float64{{-9, -10}, {43, 50}})
			second, err := transport.Evaluate(node, transport.Values(matrix.ProductInput{Left: right, Right: left}))
			So(err, ShouldBeNil)
			So(second, ShouldResemble, [][]float64{{23, 14}, {31, 18}})
			So(first, ShouldResemble, [][]float64{{-9, -10}, {43, 50}})
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
				_, err := transport.Evaluate(node, transport.Values(matrix.ProductInput{Left: operands[0], Right: operands[1]}))
				So(errors.Is(err, core.ErrShape), ShouldBeTrue)
			})
		}
	})
}
