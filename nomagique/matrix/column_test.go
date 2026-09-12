package matrix_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestColumnNext(t *testing.T) {
	Convey("Column packs a stream of scalars into an n-by-1 matrix", t, func() {
		node := matrix.NewColumn()
		vals := []float64{1.0, 2.0, 3.0}
		seq := func(yield func(unsafe.Pointer) bool) {
			for i := range vals {
				if !yield(unsafe.Pointer(&vals[i])) {
					return
				}
			}
		}
		out := tests.CollectSeq[[][]float64](node.Next(seq))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, [][]float64{{1.0}, {2.0}, {3.0}})
	})
}
