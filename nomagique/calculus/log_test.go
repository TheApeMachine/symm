package calculus

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLog(t *testing.T) {
	Convey("Given a Log primitive", t, func() {
		op := NewLog()

		Convey("Log computes natural logarithm", func() {
			got := data.Read[float64](op.Next(data.NewValue(math.E).Next(nil)))
			So(op.Error(), ShouldBeNil)
			So(got, ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("Non-positive value records domain error", func() {
			data.Read[float64](op.Next(data.NewValue(-1.0).Next(nil)))
			So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
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
