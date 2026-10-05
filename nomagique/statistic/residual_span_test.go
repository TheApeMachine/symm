package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestResidualSpanNext(t *testing.T) {
	Convey("Given a ResidualSpan primitive", t, func() {
		op := NewResidualSpan()

		Convey("tracks minimum, maximum, and span across arrivals", func() {
			in1 := ResidualSpanInput{Count: 0, Residual: 5.0}
			in2 := ResidualSpanInput{Count: 1, Minimum: 5.0, Maximum: 5.0, Residual: 2.0}
			in3 := ResidualSpanInput{Count: 2, Minimum: 2.0, Maximum: 5.0, Residual: 8.0}

			out := tests.CollectSeq[ResidualSpanResult](op.Next(tests.SliceToSeq([]ResidualSpanInput{in1, in2, in3})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(out[0].Minimum, ShouldEqual, 5.0)
			So(out[0].Maximum, ShouldEqual, 5.0)
			So(out[0].Span, ShouldEqual, 0.0)

			So(out[1].Minimum, ShouldEqual, 2.0)
			So(out[1].Maximum, ShouldEqual, 5.0)
			So(out[1].Span, ShouldEqual, 3.0)

			So(out[2].Minimum, ShouldEqual, 2.0)
			So(out[2].Maximum, ShouldEqual, 8.0)
			So(out[2].Span, ShouldEqual, 6.0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewResidualSpan()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[ResidualSpanResult](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewResidualSpan()
			in := ResidualSpanInput{Count: 0, Residual: 5.0}
			count := 0

			for range fresh.Next(tests.SliceToSeq([]ResidualSpanInput{in})) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
