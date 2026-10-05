package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCUSUMNext(t *testing.T) {
	Convey("Given a CUSUM primitive", t, func() {
		op := NewCUSUM()

		Convey("tracks directional departures and signals threshold breach", func() {
			obs1 := CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 1.0, Threshold: 5.0}
			obs2 := CUSUMObservation{Sequence: 2, Value: 104.0, Hurdle: 1.0, Threshold: 5.0}
			obs3 := CUSUMObservation{Sequence: 3, Value: 108.0, Hurdle: 1.0, Threshold: 5.0}

			out := tests.CollectSeq[CUSUMReading](op.Next(tests.SliceToSeq([]CUSUMObservation{obs1, obs2, obs3})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)

			// 1: initial baseline
			So(out[0].Signal, ShouldEqual, CUSUMNone)
			So(out[0].UpperSum, ShouldEqual, 0)

			// 2: delta = 4, hurdle = 1 -> upper = 3 < threshold 5
			So(out[1].Signal, ShouldEqual, CUSUMNone)
			So(out[1].UpperSum, ShouldEqual, 3.0)

			// 3: delta = 4, hurdle = 1 -> upper = 3 + 3 = 6 >= threshold 5 -> Signal CUSUMUpper
			So(out[2].Signal, ShouldEqual, CUSUMUpper)
		})

		Convey("signals lower threshold breach", func() {
			fresh := NewCUSUM()
			obs1 := CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 1.0, Threshold: 5.0}
			obs2 := CUSUMObservation{Sequence: 2, Value: 96.0, Hurdle: 1.0, Threshold: 5.0}
			obs3 := CUSUMObservation{Sequence: 3, Value: 92.0, Hurdle: 1.0, Threshold: 5.0}

			out := tests.CollectSeq[CUSUMReading](fresh.Next(tests.SliceToSeq([]CUSUMObservation{obs1, obs2, obs3})))

			So(fresh.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 3)
			So(out[2].Signal, ShouldEqual, CUSUMLower)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewCUSUM()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[CUSUMReading](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewCUSUM()
			obs1 := CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 1.0, Threshold: 5.0}
			count := 0

			for range fresh.Next(tests.SliceToSeq([]CUSUMObservation{obs1})) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
