package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestReturnsNext(t *testing.T) {
	Convey("Given Returns as a Primitive", t, func() {
		returns := NewReturns(core.From([]Interval(nil)))

		Convey("When I show it one run", func() {
			intervals := core.To[[]Interval](
				returns.Next(core.From(observations(4))),
			)

			So(len(intervals), ShouldEqual, 3)
			So(intervals[0].Value, ShouldAlmostEqual, math.Ln2, 1e-12)
			So(intervals[0].From, ShouldEqual, 0)
			So(intervals[0].To, ShouldEqual, int64(NanosPerSecond))
		})

		Convey("When I show it several runs in one step", func() {
			intervals := core.To[[]Interval](returns.Next(tests.NewRun(
				core.From(observations(2)),
				core.From(observations(3)),
			)))

			So(len(intervals), ShouldEqual, 3)
		})

		Convey("When time does not advance", func() {
			intervals := core.To[[]Interval](returns.Next(core.From(
				[]Observation{{Nanos: 1, Value: 1}, {Nanos: 1, Value: 2}},
			)))

			So(len(intervals), ShouldEqual, 0)
		})

		Convey("When an endpoint is not positive", func() {
			intervals := core.To[[]Interval](returns.Next(core.From(
				[]Observation{{Nanos: 1, Value: 0}, {Nanos: 2, Value: 2}},
			)))

			So(len(intervals), ShouldEqual, 0)
		})

		Convey("When a value is poisoned", func() {
			intervals := core.To[[]Interval](returns.Next(core.From(
				[]Observation{{Nanos: 1, Value: 1}, {Nanos: 2, Value: math.NaN()}},
			)))

			So(len(intervals), ShouldEqual, 1)
			So(math.IsNaN(intervals[0].Value), ShouldBeTrue)
		})

		Convey("When I offer nothing", func() {
			So(len(core.To[[]Interval](returns.Next(nil))), ShouldEqual, 0)
		})
	})
}
