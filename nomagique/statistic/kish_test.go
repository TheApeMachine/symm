package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestKishNext(t *testing.T) {
	Convey("Given a Kish primitive", t, func() {
		op := NewKish()

		Convey("computes effective sample size of weight stream", func() {
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1.0, 1.0, 1.0})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(out[0], ShouldEqual, 1.0)
			So(out[1], ShouldEqual, 2.0)
			So(out[2], ShouldEqual, 3.0)

			// Unequal weights: 2.0, 1.0 -> (2+1)^2 / (4+1) = 9/5 = 1.8
			fresh := NewKish()
			unequal := tests.CollectSeq[float64](fresh.Next(tests.SliceToSeq([]float64{2.0, 1.0})))
			So(unequal[1], ShouldAlmostEqual, 1.8, 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewKish()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewKish()
			in := []float64{1.0, 2.0, 3.0}
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

func TestKishMaturityNext(t *testing.T) {
	Convey("Given a KishMaturity primitive", t, func() {
		op := NewKishMaturity()

		Convey("maps effective sample support to maturity measure", func() {
			in := []float64{0.5, 1.0, 2.0, 4.0}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq(in)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 4)
			So(out[0], ShouldEqual, 0.0)
			So(out[1], ShouldEqual, 0.0)
			So(out[2], ShouldAlmostEqual, 0.5, 1e-9)
			So(out[3], ShouldAlmostEqual, 0.75, 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewKishMaturity()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
