package arithmetic_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOuterNext(t *testing.T) {
	Convey("Outer product computes u ⊗ v", t, func() {
		node := arithmetic.NewOuter()
		in := arithmetic.OuterInput{
			Left:  []float64{1.0, 2.0},
			Right: []float64{3.0, 4.0, 5.0},
		}
		seq := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&in))
		}
		out := tests.CollectSeq[[][]float64](node.Next(seq))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, [][]float64{
			{3.0, 4.0, 5.0},
			{6.0, 8.0, 10.0},
		})
	})
}
