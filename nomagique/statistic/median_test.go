package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMedianNext(t *testing.T) {
	Convey("Given a Median primitive", t, func() {
		Convey("averages the central order statistics of an odd run", func() {
			med := NewMedian()
			out := tests.CollectSeq[float64](med.Next(tests.SliceToSeq([]float64{9.0, 1.0, 5.0})))

			So(med.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, 5.0)
		})

		Convey("averages the central order statistics of an even run", func() {
			med := NewMedian()
			out := tests.CollectSeq[float64](med.Next(tests.SliceToSeq([]float64{9.0, 1.0, 5.0, 3.0})))

			So(med.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, 4.0)
		})

		Convey("empty input reports ErrShape", func() {
			med := NewMedian()
			out := tests.CollectSeq[float64](med.Next(tests.SliceToSeq([]float64{})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(med.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			med := NewMedian()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](med.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(med.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
