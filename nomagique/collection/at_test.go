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

func TestAtNext(t *testing.T) {
	Convey("At selects a configured index from each arriving collection", t, func() {
		op := collection.NewAt[float64](1)
		vals := []float64{3, 4, 5}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&vals))
		}
		out := tests.CollectSeq[float64](op.Next(in))

		So(out, ShouldResemble, []float64{4})
		So(op.Error(), ShouldBeNil)
	})

	Convey("At records a shape error for an index outside the collection", t, func() {
		op := collection.NewAt[float64](3)
		vals := []float64{3, 4, 5}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&vals))
		}
		out := tests.CollectSeq[float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}
