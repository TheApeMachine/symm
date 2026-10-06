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

func TestWithoutKeyNext(t *testing.T) {
	Convey("WithoutKey drops the configured key and leaves the input alone", t, func() {
		op := collection.NewWithoutKey[string, float64]("b")
		values := map[string]float64{"a": 1, "b": 2, "c": 3}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&values))
		}
		out := tests.CollectSeq[map[string]float64](op.Next(in))

		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, map[string]float64{"a": 1, "c": 3})
		So(values, ShouldResemble, map[string]float64{"a": 1, "b": 2, "c": 3})
		So(op.Error(), ShouldBeNil)
	})

	Convey("WithoutKey records a shape error for a nil arrival", t, func() {
		op := collection.NewWithoutKey[string, float64]("b")
		in := func(yield func(unsafe.Pointer) bool) {
			yield(nil)
		}
		out := tests.CollectSeq[map[string]float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}
