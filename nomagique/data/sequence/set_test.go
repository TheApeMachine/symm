package sequence_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSetNext(t *testing.T) {
	Convey("Set replaces one indexed member without mutating the input", t, func() {
		original := []float64{2, 3, 4}
		op := sequence.NewSet(1, 7.0)
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&original))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{2, 7, 4})
		So(original[1], ShouldEqual, 3)
		So(op.Error(), ShouldBeNil)
	})

	Convey("Set records a shape error for an index outside the collection", t, func() {
		op := sequence.NewSet(3, 8.0)
		vals := []float64{1, 2, 3}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&vals))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}
