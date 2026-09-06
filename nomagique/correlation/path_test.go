package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
observations is the run every case in this package is driven by: a path that
doubles every second, so its returns, spacings, and energies are all known
without restating the arithmetic that produces them.
*/
func observations(count int) []Observation {
	run := make([]Observation, 0, count)

	for index := range count {
		run = append(run, Observation{
			Nanos: int64(index) * int64(NanosPerSecond),
			Value: math.Pow(2, float64(index)),
		})
	}

	return run
}

func TestPathNext(t *testing.T) {
	Convey("Given a Path as a Primitive", t, func() {
		path := NewPath(core.From([]Observation(nil)))

		Convey("When I observe one run", func() {
			retained := core.To[[]Observation](
				path.Next(core.From(observations(3))),
			)

			So(len(retained), ShouldEqual, 3)
			So(retained[2].Value, ShouldEqual, 4)
		})

		Convey("When I observe several runs in one step", func() {
			retained := core.To[[]Observation](path.Next(tests.NewRun(
				core.From(observations(2)),
				core.From([]Observation{{Nanos: 5 * int64(NanosPerSecond), Value: 9}}),
			)))

			So(len(retained), ShouldEqual, 3)
			So(retained[2].Value, ShouldEqual, 9)
		})

		Convey("When I observe across steps", func() {
			path.Next(core.From(observations(2)))
			retained := core.To[[]Observation](path.Next(
				core.From([]Observation{{Nanos: 9 * int64(NanosPerSecond), Value: 7}}),
			))

			So(len(retained), ShouldEqual, 3)
		})

		Convey("When an observation regresses in time", func() {
			path.Next(core.From(observations(3)))
			retained := core.To[[]Observation](path.Next(
				core.From([]Observation{{Nanos: 0, Value: 99}}),
			))

			So(len(retained), ShouldEqual, 3)
			So(retained[0].Value, ShouldEqual, 1)
		})

		Convey("When an observation repeats a timestamp", func() {
			path.Next(core.From(observations(3)))
			retained := core.To[[]Observation](path.Next(
				core.From([]Observation{{
					Nanos: 2 * int64(NanosPerSecond), Value: 99,
				}}),
			))

			So(len(retained), ShouldEqual, 3)
			So(retained[2].Value, ShouldEqual, 99)
		})

		Convey("When I observe a poisoned value", func() {
			retained := core.To[[]Observation](path.Next(
				core.From([]Observation{{Nanos: 1, Value: math.NaN()}}),
			))

			So(math.IsNaN(retained[0].Value), ShouldBeTrue)
		})

		Convey("When I offer nothing", func() {
			path.Next(core.From(observations(3)))

			So(len(core.To[[]Observation](path.Next(nil))), ShouldEqual, 3)
		})
	})
}
