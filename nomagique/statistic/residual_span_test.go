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
			in1 := [4]float64{0, 0, 0, 5.0}
			in2 := [4]float64{1, 5.0, 5.0, 2.0}
			in3 := [4]float64{2, 2.0, 5.0, 8.0}

			out := tests.CollectSeq[[4]float64](op.Next(tests.SliceToSeq([][4]float64{in1, in2, in3})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(out[0][1], ShouldEqual, 5.0)
			So(out[0][2], ShouldEqual, 5.0)
			So(out[0][3], ShouldEqual, 0.0)

			So(out[1][1], ShouldEqual, 2.0)
			So(out[1][2], ShouldEqual, 5.0)
			So(out[1][3], ShouldEqual, 3.0)

			So(out[2][1], ShouldEqual, 2.0)
			So(out[2][2], ShouldEqual, 8.0)
			So(out[2][3], ShouldEqual, 6.0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewResidualSpan()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[[4]float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewResidualSpan()
			in := [4]float64{0, 0, 0, 5.0}
			count := 0

			for range fresh.Next(tests.SliceToSeq([][4]float64{in})) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
