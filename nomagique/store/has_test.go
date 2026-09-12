package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestHasNext(t *testing.T) {
	Convey("Has reports membership of a configured key", t, func() {
		op := store.NewHas[string, float64]("x")
		m1 := map[string]float64{"x": 0}
		m2 := map[string]float64{}
		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&m1)) {
				return
			}

			yield(unsafe.Pointer(&m2))
		}
		out := tests.CollectSeq[bool](op.Next(in))

		So(out, ShouldResemble, []bool{true, false})
		So(op.Error(), ShouldBeNil)
	})
}
