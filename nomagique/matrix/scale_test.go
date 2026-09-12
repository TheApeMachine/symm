package matrix_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestScaleNext(t *testing.T) {
	Convey("Scale multiplies every coefficient without mutating the source", t, func() {
		node := matrix.NewScale()
		values := [][]float64{{1, -2}, {}, {3}}
		in1 := matrix.ScaleInput{Values: values, Factor: -2}
		seq1 := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in1))
		}
		first := tests.CollectSeq[[][]float64](node.Next(seq1))
		So(node.Error(), ShouldBeNil)
		So(first[0], ShouldResemble, [][]float64{{-2, 4}, {}, {-6}})

		node2 := matrix.NewScale()
		in2 := matrix.ScaleInput{Values: values, Factor: 0}
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
