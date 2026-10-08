package temporal_test

import (
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestInterval(t *testing.T) {
	Convey("IntervalOverlap reports intersection for half-open intervals", t, func() {
		op := temporal.NewIntervalOverlap()
		pair1 := temporal.IntervalPair{
			Left:  temporal.Interval{From: 10, To: 20},
			Right: temporal.Interval{From: 15, To: 25},
		}
		pair2 := temporal.IntervalPair{
			Left:  temporal.Interval{From: 10, To: 20},
			Right: temporal.Interval{From: 20, To: 30},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair1))
			yield(unsafe.Pointer(&pair2))
		}
		out := tests.CollectSeq[bool](op.Next(in))
		So(out, ShouldResemble, []bool{true, false})
	})

	Convey("IntervalJoin finds overlapping pairs in O(N+M)", t, func() {
		op := temporal.NewIntervalJoin()
		input := temporal.IntervalJoinInput{
			Left: []temporal.Interval{
				{From: 0, To: 10},
				{From: 10, To: 20},
				{From: 25, To: 35},
			},
			Right: []temporal.Interval{
				{From: 5, To: 15},
				{From: 20, To: 30},
			},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&input))
		}
		out := tests.CollectSeq[temporal.IntervalPair](op.Next(in))
		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldResemble, temporal.IntervalPair{
			Left:  temporal.Interval{From: 0, To: 10},
			Right: temporal.Interval{From: 5, To: 15},
		})
		So(out[1], ShouldResemble, temporal.IntervalPair{
			Left:  temporal.Interval{From: 10, To: 20},
			Right: temporal.Interval{From: 5, To: 15},
		})
		So(out[2], ShouldResemble, temporal.IntervalPair{
			Left:  temporal.Interval{From: 25, To: 35},
			Right: temporal.Interval{From: 20, To: 30},
		})
	})

	Convey("Elapsed converts nanoseconds to seconds", t, func() {
		op := temporal.NewElapsed()
		interval := temporal.Interval{From: 1_000_000_000, To: 3_500_000_000}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&interval))
		}
		out := tests.CollectSeq[float64](op.Next(in))
		So(out, ShouldResemble, []float64{2.5})
	})

	Convey("EnergyRates computes r^2 / elapsed", t, func() {
		op := temporal.NewEnergyRates()
		input := temporal.EnergyRateInput{
			Value: 2.0,
			From:  0,
			To:    2 * int64(time.Second),
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&input))
		}
		out := tests.CollectSeq[float64](op.Next(in))
		So(out, ShouldResemble, []float64{2.0})
	})
}
