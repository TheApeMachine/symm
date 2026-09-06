package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCountNext(t *testing.T) {
	Convey("Given a Count as a Primitive", t, func() {
		count := NewCount(core.From(0.0))

		Convey("When I show it one run", func() {
			So(core.To[float64](count.Next(core.From([]float64{1, 2, 3}))),
				ShouldEqual, 3)
		})

		Convey("When I show it several runs in one step", func() {
			So(core.To[float64](count.Next(tests.NewRun(
				core.From([]float64{1, 2}),
				core.From([]float64{3}),
			))), ShouldEqual, 3)
		})

		Convey("When I show it an empty run", func() {
			So(core.To[float64](count.Next(core.From([]float64{}))), ShouldEqual, 0)
		})

		Convey("When I offer nothing", func() {
			So(core.To[float64](count.Next(nil)), ShouldEqual, 0)
		})
	})
}
