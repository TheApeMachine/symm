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

func TestVectorNext(t *testing.T) {
	Convey("Vector multiplies a matrix by a vector", t, func() {
		node := matrix.NewVector()
		in := matrix.VectorInput{
			Matrix: [][]float64{
				{1, 2},
				{3, 4},
			},
			Vector: []float64{5, 6},
		}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in))
		}
		out := tests.CollectSeq[[]float64](node.Next(seq))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{1*5 + 2*6, 3*5 + 4*6}) // 17, 39
	})

	Convey("Mismatched dimensions return ErrShape", t, func() {
		node := matrix.NewVector()
		in := matrix.VectorInput{
			Matrix: [][]float64{
				{1, 2, 3},
			},
			Vector: []float64{5, 6},
		}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in))
		}
		tests.CollectSeq[[]float64](node.Next(seq))
		So(errors.Is(node.Error(), core.ErrShape), ShouldBeTrue)
	})
}
