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

func TestCoefficientSNRNext(t *testing.T) {
	Convey("Given a CoefficientSNR primitive", t, func() {
		op := NewCoefficientSNR()

		Convey("computes Coefficient^2 / Variance", func() {
			in := [2]float64{2.0, 0.5}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([][2]float64{in})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, 8.0)
		})

		Convey("returns NaN when variance is non-positive or NaN", func() {
			in := [][2]float64{
				{2.0, 0.0},
				{2.0, -1.0},
				{2.0, math.NaN()},
			}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq(in)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(math.IsNaN(out[0]), ShouldBeTrue)
			So(math.IsNaN(out[1]), ShouldBeTrue)
			So(math.IsNaN(out[2]), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewCoefficientSNR()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewCoefficientSNR()
			in := [][2]float64{
				{2.0, 0.5},
				{4.0, 2.0},
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
