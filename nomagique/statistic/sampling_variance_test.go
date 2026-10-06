package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSamplingVarianceNext(t *testing.T) {
	Convey("Given a SamplingVariance primitive", t, func() {
		op := NewSamplingVariance()

		Convey("applies specificity debt floor", func() {
			in := [4]float64{2.0, 4.0, 10.0, 3.0}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([][4]float64{in})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			// floor = 10 / (1 + 2) = 10/3. variance / floor = 3 / (10/3) = 0.9
			So(out[0], ShouldAlmostEqual, 0.9, 1e-9)
		})

		Convey("depth exceeding context length records ErrDomain", func() {
			in := [4]float64{5.0, 4.0, 10.0, 3.0}
			out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([][4]float64{in})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewSamplingVariance()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewSamplingVariance()
			inputs := [][4]float64{
				{1.0, 4.0, 10.0, 2.0},
				{2.0, 4.0, 10.0, 3.0},
			}
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
