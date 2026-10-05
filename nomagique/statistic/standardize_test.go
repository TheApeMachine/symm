package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestStandardizeNext(t *testing.T) {
	Convey("Given a Standardize primitive", t, func() {
		Convey("Standardize with fixed parameters", func() {
			std := NewStandardize(10.0, 2.0)
			out := tests.CollectSeq[float64](std.Next(tests.SliceToSeq([]float64{10.0, 12.0, 8.0})))

			So(std.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(out[0], ShouldEqual, 0.0)
			So(out[1], ShouldEqual, 1.0)
			So(out[2], ShouldEqual, -1.0)
		})

		Convey("Standardize with per-arrival input", func() {
			std := NewStandardize()
			in := []StandardizeInput{
				{Value: 15, Center: 10, Scale: 5},
				{Value: 5, Center: 10, Scale: 5},
			}
			out := tests.CollectSeq[float64](std.Next(tests.SliceToSeq(in)))

			So(std.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, 1.0)
			So(out[1], ShouldEqual, -1.0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewStandardize(10.0, 2.0)
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewStandardize(10.0, 2.0)
			in := []float64{10.0, 12.0, 8.0}
			count := 0

			for range fresh.Next(tests.SliceToSeq(in)) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
