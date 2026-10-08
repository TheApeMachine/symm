package calculus

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestMinimum(t *testing.T) {
	Convey("Given a Minimum primitive", t, func() {
		op := NewMinimum()

		Convey("It yields the minimum across arrivals in a run", func() {
			got := data.Read[float64](op.Next(data.NewValue(5.0, 3.0, 8.0, 1.0, 4.0).Next(nil)))
			So(op.Error(), ShouldBeNil)
			So(got, ShouldEqual, 1.0)
		})

		Convey("Single arrival yields that value", func() {
			got := data.Read[float64](op.Next(data.NewValue(42.0).Next(nil)))
			So(op.Error(), ShouldBeNil)
			So(got, ShouldEqual, 42.0)
		})

		Convey("When a nil pointer arrives, it records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}

			data.Read[float64](op.Next(nilSeq))
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
