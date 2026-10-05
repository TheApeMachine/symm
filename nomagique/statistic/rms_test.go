package statistic

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRMSNext(t *testing.T) {
	Convey("Given an RMS primitive", t, func() {
		op := NewRMS()

		Convey("computes sqrt(sum(x^2)/n)", func() {
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{3.0, 4.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, 3.0)
			So(out[1], ShouldAlmostEqual, math.Sqrt(12.5), 1e-9)
		})

		Convey("continues accumulation on subsequent runs", func() {
			_ = tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{3.0, 4.0})))
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{5.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			// (9 + 16 + 25) / 3 = 50 / 3 ≈ 16.6666667
			So(out[0], ShouldAlmostEqual, math.Sqrt(50.0/3.0), 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewRMS()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewRMS()
			in := tests.SliceToSeq([]float64{3.0, 4.0, 5.0})
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
