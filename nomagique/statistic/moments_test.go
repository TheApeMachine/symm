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
			out := tests.CollectSeq[[10]float64](op.Next(tests.SliceToSeq(inputs)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 8)

			last := out[len(out)-1]
			So(last[0], ShouldEqual, 8)
			So(last[1], ShouldEqual, 5.0)
			So(last[8], ShouldAlmostEqual, 32.0/7.0, 1e-12)
			So(last[9], ShouldAlmostEqual, math.Sqrt(32.0/7.0), 1e-12)
		})

		Convey("leaves variance and dispersion undefined (zero) on the first observation", func() {
			fresh := NewEstimator()
			out := tests.CollectSeq[[10]float64](fresh.Next(tests.SliceToSeq([]float64{7.0})))

			So(fresh.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][0], ShouldEqual, 1)
			So(out[0][1], ShouldEqual, 7.0)
			So(out[0][8], ShouldEqual, 0)
			So(out[0][9], ShouldEqual, 0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewEstimator()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[[10]float64](fresh.Next(nilSeq))

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

func TestShedNext(t *testing.T) {
	Convey("Given a Shed primitive over an Estimator", t, func() {
		est := NewEstimator()

		for range est.Next(tests.SliceToSeq([]float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0})) {
		}

		shed := NewShed(est)

		Convey("halves support while preserving the Bessel-corrected variance", func() {
			out := tests.CollectSeq[[3]float64](shed.Next(tests.SliceToSeq([]float64{0.5})))

			So(shed.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][0], ShouldEqual, 4)
			So(out[0][1], ShouldEqual, 5.0)
			So(out[0][2]/(out[0][0]-1), ShouldAlmostEqual, 32.0/7.0, 1e-12)
		})

		Convey("ignores ratios outside (0, 1)", func() {
			out := tests.CollectSeq[[3]float64](shed.Next(tests.SliceToSeq([]float64{1.0, 0.0})))

			So(len(out), ShouldEqual, 2)
			So(out[1][0], ShouldEqual, 8)
		})

		Convey("floors support at two samples", func() {
			out := tests.CollectSeq[[3]float64](shed.Next(tests.SliceToSeq([]float64{0.01})))

			So(out[0][0], ShouldEqual, 2)
		})
	})
}
