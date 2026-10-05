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
			obs1 := PriorObservation{
				Value:     10.0,
				Authority: 1.0,
				Memory:    10.0,
			}
			obs2 := PriorObservation{
				Value:     20.0,
				Authority: 1.0,
				Memory:    10.0,
			}

			out := tests.CollectSeq[PriorSummary](op.Next(tests.SliceToSeq([]PriorObservation{obs1, obs2})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			So(out[0].Samples, ShouldEqual, 1)
			So(out[0].Mean, ShouldEqual, 10.0)
			So(out[0].Defined, ShouldBeTrue)

			So(out[1].Samples, ShouldEqual, 2)
			So(out[1].Mean, ShouldAlmostEqual, 10.0+10.0/1.9, 1e-9)
		})

		Convey("age-only query does not increment samples", func() {
			query := PriorObservation{
				AgeOnly:  true,
				Epoch:    5,
				HasEpoch: true,
				Memory:   10.0,
			}
			out := tests.CollectSeq[PriorSummary](op.Next(tests.SliceToSeq([]PriorObservation{query})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0].Samples, ShouldEqual, 0)
		})

		Convey("invalid authority records ErrDomain", func() {
			invalid := PriorObservation{
				Value:     10.0,
				Authority: 2.0,
				Memory:    10.0,
			}
			out := tests.CollectSeq[PriorSummary](op.Next(tests.SliceToSeq([]PriorObservation{invalid})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewPriorEstimator()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[PriorSummary](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewPriorEstimator()
			obs := PriorObservation{
				Value:     10.0,
				Authority: 1.0,
				Memory:    10.0,
			}
			count := 0

			for range fresh.Next(tests.SliceToSeq([]PriorObservation{obs})) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
