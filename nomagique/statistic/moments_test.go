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

func TestMomentsNext(t *testing.T) {
	Convey("Given an Estimator primitive", t, func() {
		op := NewEstimator()

		Convey("computes running Welford moments", func() {
			inputs := []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}
			out := tests.CollectSeq[MomentReading](op.Next(tests.SliceToSeq(inputs)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 8)

			last := out[len(out)-1]
			So(last.Count, ShouldEqual, 8)
			So(last.Mean, ShouldEqual, 5.0)
			So(last.Variance, ShouldAlmostEqual, 32.0/7.0, 1e-12)
			So(last.Dispersion, ShouldAlmostEqual, math.Sqrt(32.0/7.0), 1e-12)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewEstimator()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[MomentReading](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewEstimator()
			inputs := []float64{2.0, 4.0, 6.0}
			count := 0

			for range fresh.Next(tests.SliceToSeq(inputs)) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
