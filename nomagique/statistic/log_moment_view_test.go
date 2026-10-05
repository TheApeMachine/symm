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

func TestLogMomentViewNext(t *testing.T) {
	Convey("Given a LogMomentView primitive", t, func() {
		op := NewLogMomentView()

		Convey("reads log-space Welford records against prior moments", func() {
			est := NewEstimator()
			var readings []MomentReading

			// log(10) ≈ 2.302585, log(20) ≈ 2.995732
			for out := range est.Next(tests.SliceToSeq([]float64{math.Log(10.0), math.Log(20.0)})) {
				readings = append(readings, *(*MomentReading)(out))
			}

			out := tests.CollectSeq[CausalResidualResult](op.Next(tests.SliceToSeq(readings)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)

			// First reading has no prior
			So(out[0].HasPrior, ShouldBeFalse)

			// Second reading has prior from first
			So(out[1].HasPrior, ShouldBeTrue)
			So(out[1].Baseline, ShouldAlmostEqual, 10.0, 1e-9)
			So(out[1].Residual, ShouldAlmostEqual, math.Log(20.0)-math.Log(10.0), 1e-9)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewLogMomentView()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[CausalResidualResult](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewLogMomentView()
			est := NewEstimator()
			var readings []MomentReading

			for out := range est.Next(tests.SliceToSeq([]float64{1.0, 2.0})) {
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
