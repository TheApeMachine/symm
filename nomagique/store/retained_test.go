package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRetainedNext(t *testing.T) {
	Convey("Retained holds the latest arrival and yields it on Next", t, func() {
		memory := store.NewRetained(10.0)
		val := 20.0
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&val))
		}
		out := tests.CollectSeq[float64](memory.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldEqual, 20.0)

		outHeld := tests.CollectSeq[float64](memory.Next(nil))
		So(len(outHeld), ShouldEqual, 1)
		So(outHeld[0], ShouldEqual, 20.0)
	})
}
