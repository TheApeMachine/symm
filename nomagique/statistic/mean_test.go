package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMeanNext(t *testing.T) {
	Convey("Given a Mean primitive", t, func() {
		op := NewMean()

		Convey("computes the running arithmetic mean", func() {
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1.0, 2.0, 3.0, 4.0, 5.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 5)
			So(out[0], ShouldEqual, 1.0)
			So(out[1], ShouldEqual, 1.5)
			So(out[2], ShouldEqual, 2.0)
			So(out[3], ShouldEqual, 2.5)
			So(out[4], ShouldEqual, 3.0)
		})

		Convey("continues accumulation on subsequent runs", func() {
			_ = tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1.0, 2.0, 3.0, 4.0, 5.0})))
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{6.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldAlmostEqual, 3.5, 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewMean()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewMean()
			in := tests.SliceToSeq([]float64{1.0, 2.0, 3.0})
			count := 0

			for range fresh.Next(in) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
