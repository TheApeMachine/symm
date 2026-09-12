package collection_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSwapNext(t *testing.T) {
	Convey("Swap exchanges two configured indices without mutating the input", t, func() {
		op := collection.NewSwap[float64](0, 2)
		original := []float64{2, 3, 4}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&original))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{4, 3, 2})
		So(original[0], ShouldEqual, 2)
		So(op.Error(), ShouldBeNil)
	})
}
