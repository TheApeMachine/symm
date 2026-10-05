package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCausalResidualNext(t *testing.T) {
	Convey("Given a CausalResidual primitive", t, func() {
		op := NewCausalResidual()

		Convey("evaluates residuals against prior moments", func() {
			est := NewEstimator()
			var readings []MomentReading

			for out := range est.Next(tests.SliceToSeq([]float64{10.0, 20.0})) {
				readings = append(readings, *(*MomentReading)(out))
			}

			out := tests.CollectSeq[CausalResidualResult](op.Next(tests.SliceToSeq(readings)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)

			// First reading: no prior
			So(out[0].HasPrior, ShouldBeFalse)
			So(out[0].Baseline, ShouldEqual, 10.0)
			So(out[0].Residual, ShouldEqual, 0.0)

			// Second reading: has prior from first reading
			So(out[1].HasPrior, ShouldBeTrue)
			So(out[1].Baseline, ShouldEqual, 10.0)
			So(out[1].Residual, ShouldEqual, 10.0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewCausalResidual()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[CausalResidualResult](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewCausalResidual()
			est := NewEstimator()
			var readings []MomentReading

			for out := range est.Next(tests.SliceToSeq([]float64{10.0, 20.0})) {
				readings = append(readings, *(*MomentReading)(out))
			}

			count := 0

			for range fresh.Next(tests.SliceToSeq(readings)) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
