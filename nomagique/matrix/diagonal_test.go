package matrix_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDiagonalNext(t *testing.T) {
	Convey("Diagonal extracts the main diagonal elements", t, func() {
		node := matrix.NewDiagonal()
		mat := [][]float64{
			{1, 2, 3},
			{4, 5, 6},
			{7, 8, 9},
		}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&mat))
		}
		out := tests.CollectSeq[[]float64](node.Next(seq))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{1, 5, 9})
	})

	Convey("Undersized row returns ErrShape", t, func() {
		node := matrix.NewDiagonal()
		mat := [][]float64{
			{1, 2},
			{4},
		}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&mat))
		}
		tests.CollectSeq[[]float64](node.Next(seq))
		So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
	})
}
