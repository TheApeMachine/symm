package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSpacingsNext(t *testing.T) {
	Convey("Given Spacings as a Primitive", t, func() {
		spacings := NewSpacings(core.From([]float64(nil)))

		Convey("When I show it one run", func() {
			gaps := core.To[[]float64](
				spacings.Next(core.From(observations(4))),
			)

			So(len(gaps), ShouldEqual, 3)
			So(gaps[0], ShouldEqual, NanosPerSecond)
		})

		Convey("When I show it several runs in one step", func() {
			gaps := core.To[[]float64](spacings.Next(tests.NewRun(
				core.From(observations(2)),
				core.From(observations(4)),
			)))

			So(len(gaps), ShouldEqual, 4)
		})

		Convey("When time does not advance", func() {
			gaps := core.To[[]float64](spacings.Next(core.From(
				[]Observation{{Nanos: 7, Value: 1}, {Nanos: 7, Value: 2}},
			)))

			So(len(gaps), ShouldEqual, 0)
		})

		Convey("When I offer nothing", func() {
			So(len(core.To[[]float64](spacings.Next(nil))), ShouldEqual, 0)
		})
	})
}
