package collection_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestTailNext(t *testing.T) {
	Convey("Tail keeps the last configured members of each arrival", t, func() {
		op := collection.NewTail[float64](2)
		values := []float64{1, 2, 3}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&values))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{2, 3})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Tail records a shape error for a negative capacity and yields nothing", t, func() {
		op := collection.NewTail[float64](-1)
		values := []float64{1, 2, 3}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&values))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}
