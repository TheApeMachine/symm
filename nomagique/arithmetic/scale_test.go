package arithmetic_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMatrixScaleNext(t *testing.T) {
	Convey("MatrixScale multiplies every coefficient without mutating the source", t, func() {
		node := arithmetic.NewMatrixScale()
		values := [][]float64{{1, -2}, {}, {3}}
		in1 := arithmetic.MatrixScaleInput{Values: values, Factor: -2}
		seq1 := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in1))
		}
		first := tests.CollectSeq[[][]float64](node.Next(seq1))
		So(node.Error(), ShouldBeNil)
		So(first[0], ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})

		node2 := arithmetic.NewMatrixScale()
		in2 := arithmetic.MatrixScaleInput{Values: values, Factor: 0}
		seq2 := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in2))
		}
		second := tests.CollectSeq[[][]float64](node2.Next(seq2))
		So(node2.Error(), ShouldBeNil)
		So(second[0], ShouldResemble, [][]float64{{0, 0}, {}, {0}})
		So(first[0], ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})
		So(values, ShouldResemble, [][]float64{{1, -2}, {}, {3}})
	})
}
