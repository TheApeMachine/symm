package collection_test

import (
	"slices"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOrderNext(t *testing.T) {
	Convey("Order sorts a clone and leaves the input collection alone", t, func() {
		input := []float64{3, 1, 2}
		op := collection.NewOrder[float64]()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&input))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{1, 2, 3})
		So(slices.Equal(input, []float64{3, 1, 2}), ShouldBeTrue)
		So(op.Error(), ShouldBeNil)
	})
}
