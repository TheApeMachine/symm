package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPriorEstimatorNext(t *testing.T) {
	Convey("Given a PriorEstimator primitive", t, func() {
		op := NewPriorEstimator()

		Convey("applies observation and updates recurrence", func() {
			obs1 := [6]float64{10.0, 1.0, 10.0, 0, 0, 0}
			obs2 := [6]float64{20.0, 1.0, 10.0, 0, 0, 0}

			out := tests.CollectSeq[[11]float64](op.Next(tests.SliceToSeq([][6]float64{obs1, obs2})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0][0], ShouldEqual, 1)
			So(out[0][4], ShouldEqual, 10.0)
			So(out[0][2], ShouldEqual, 1)

			So(out[1][0], ShouldEqual, 2)
			So(out[1][4], ShouldAlmostEqual, 10.0+10.0/1.9, 1e-9)
		})

		Convey("age-only query does not increment samples", func() {
			query := [6]float64{0, 0, 10.0, 5, 1, 1}
			out := tests.CollectSeq[[11]float64](op.Next(tests.SliceToSeq([][6]float64{query})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][0], ShouldEqual, 0)
		})

		Convey("invalid authority records ErrDomain", func() {
			invalid := [6]float64{10.0, 2.0, 10.0, 0, 0, 0}
			out := tests.CollectSeq[[11]float64](op.Next(tests.SliceToSeq([][6]float64{invalid})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewPriorEstimator()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[[11]float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewPriorEstimator()
			obs := [6]float64{10.0, 1.0, 10.0, 0, 0, 0}
			count := 0

			for range fresh.Next(tests.SliceToSeq([][6]float64{obs})) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
