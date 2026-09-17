package transport_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestOnceNext(t *testing.T) {
	Convey("A prerequisite completes once before input is passed downstream", t, func() {
		reading := store.NewRetained(statistic.MomentReading{})
		once := transport.NewOnce(nomagique.NewNumber(
			sequence.NewValues(1.0, 2.0, 3.0), statistic.NewEstimator(), reading,
		))

		Convey("Construction and empty input leave the prerequisite idle", func() {
			So(tests.CollectSeq[int](once.Next(sequence.NewValue[int]())), ShouldBeEmpty)
			So(sequence.Read[statistic.MomentReading](reading.Next(nil)).Count, ShouldEqual, 0)
		})

		Convey("Repeated streams preserve every arrival and do not repeat setup", func() {
			for _, values := range [][]int{{1, 2, 3}, {}, {4, 5}} {
				for index, output := range tests.CollectSeq[int](once.Next(sequence.NewValue(values...))) {
					So(output, ShouldEqual, values[index])
					So(sequence.Read[statistic.MomentReading](reading.Next(nil)).Count, ShouldEqual, 3)
				}
			}
			So(once.Error(), ShouldBeNil)
		})

		Convey("Stopping early propagates cancellation without repeating setup later", func() {
			for range once.Next(sequence.NewValue(1, 2, 3)) {
				break
			}
			So(tests.CollectSeq[int](once.Next(sequence.NewValue(4))), ShouldResemble, []int{4})
			So(sequence.Read[statistic.MomentReading](reading.Next(nil)).Count, ShouldEqual, 3)
		})
	})

	Convey("A failing prerequisite prevents input delivery", t, func() {
		prerequisite := nomagique.NewNumber(
			sequence.NewValues(map[string]float64{"one": 1.0, "two": 2.0, "three": 3.0}),
			store.NewGet[string, float64]("missing"),
		)
		once := transport.NewOnce(prerequisite)
		So(tests.CollectSeq[int](once.Next(sequence.NewValue(1, 2))), ShouldBeEmpty)
		So(errors.Is(once.Error(), core.ErrNotHeld), ShouldBeTrue)
		So(tests.CollectSeq[int](once.Next(sequence.NewValue(3))), ShouldBeEmpty)
	})
}

func BenchmarkOnceNext(b *testing.B) {
	once := transport.NewOnce(sequence.NewValues(1))
	for range once.Next(sequence.NewValue(1)) {
	}
	b.ReportAllocs()
	
	for index := 0; b.Loop(); index++ {
		for range once.Next(sequence.NewValue(index)) {
		}
	}
}
