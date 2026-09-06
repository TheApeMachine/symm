package core_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
A Proto carries the value it was constructed with and ignores whatever steps
it, so driving one across a range asserts it stays that constant. It is not
fuzzed: a carrier cannot propagate a poison it never accepts.
*/
func TestProtoNext(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"float64", "hold", core.From(7.0),
			tests.WithGenerator[float64](7, 0, 10, false),
		),
		tests.NewTestCase(
			"float64", "hold", core.From(-3.0),
			tests.WithGenerator[float64](-3, -10, 0, false),
		),
	).Run(t)
}

/*
A carrier has to end its delivery run, or nothing could ever drain it: a fold
keeps asking until it is told there is nothing more. The run belongs to the
caller, so a second caller is handed the value again rather than the end of
somebody else's pass.
*/
func TestProtoYield(t *testing.T) {
	Convey("Given a carrier drained through Yield", t, func() {
		carrier := core.From(2.0)

		Convey("When one fold drains it", func() {
			So(core.To[float64](core.Yield(
				core.From(0.0), carrier,
				func(held, value float64) float64 { return held + value },
			)), ShouldEqual, 2)
		})

		Convey("When a second fold drains it afterwards", func() {
			core.Yield(core.From(0.0), carrier,
				func(held, value float64) float64 { return held + value })

			So(core.To[float64](core.Yield(
				core.From(0.0), carrier,
				func(held, value float64) float64 { return held + value },
			)), ShouldEqual, 2)
		})

		Convey("When there is nothing to drain", func() {
			So(core.To[float64](core.Yield(
				core.From(7.0), nil,
				func(held, value float64) float64 { return held + value },
			)), ShouldEqual, 7)
		})
	})
}
