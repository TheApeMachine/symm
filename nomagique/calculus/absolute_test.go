package calculus

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAbsolute(t *testing.T) {
	Convey("Given an Absolute primitive", t, func() {
		op := NewAbsolute()

		Convey("Computes absolute value", func() {
			got := data.Read[float64](op.Next(data.NewValue(-42.0).Next(nil)))
			So(op.Error(), ShouldBeNil)
			So(got, ShouldEqual, 42.0)
		})

		Convey("Nil arrival records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			data.Read[float64](op.Next(nilSeq))
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
