package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestWeightedMeanNext(t *testing.T) {
	Convey("Given a WeightedMean primitive", t, func() {
		op := NewWeightedMean()

		Convey("computes sum(w x) / sum(w)", func() {
			in := [][2]float64{
				{1.0, 10.0},
				{2.0, 20.0},
			}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq(in)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, 10.0)
			So(out[1], ShouldAlmostEqual, 50.0/3.0, 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewWeightedMean()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewWeightedMean()
			in := [][2]float64{
				{1.0, 10.0},
				{2.0, 20.0},
			}
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

func TestWeightedVarianceNext(t *testing.T) {
	Convey("Given a WeightedVariance primitive", t, func() {
		op := NewWeightedVariance()

		Convey("computes E_w[x^2] - E_w[x]^2", func() {
			in := [][2]float64{
				{1.0, 10.0},
				{1.0, 20.0},
			}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq(in)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0], ShouldEqual, 0.0)
			So(out[1], ShouldAlmostEqual, 25.0, 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewWeightedVariance()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
