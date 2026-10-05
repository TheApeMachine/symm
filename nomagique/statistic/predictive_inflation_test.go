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

func TestPredictiveInflationNext(t *testing.T) {
	Convey("Given a PredictiveInflation primitive", t, func() {
		op := NewPredictiveInflation()

		Convey("computes sqrt(1 + 1/count)", func() {
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1.0, 3.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldAlmostEqual, math.Sqrt(2.0), 1e-9)
			So(out[1], ShouldAlmostEqual, math.Sqrt(4.0/3.0), 1e-9)
		})

		Convey("non-positive count records ErrDomain", func() {
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{0.0})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewPredictiveInflation()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewPredictiveInflation()
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
