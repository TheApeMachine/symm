package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAccumulator(t *testing.T) {
	Convey("Given an Accumulator with no Source", t, func() {
		accumulator := &Accumulator{}

		Convey("it accumulates the carrier itself", func() {
			So(accumulator.Step(types.Scalar(2)), ShouldEqual, types.Scalar(2))
			So(accumulator.Step(types.Scalar(3)), ShouldEqual, types.Scalar(5))
			So(accumulator.Step(types.Scalar(-1)), ShouldEqual, types.Scalar(4))
		})

		Convey("Total reads the sum without advancing it", func() {
			accumulator.Step(types.Scalar(7))
			So(accumulator.Total(), ShouldEqual, types.Scalar(7))
			So(accumulator.Total(), ShouldEqual, types.Scalar(7))
		})
	})

	Convey("Given an Accumulator over a Source", t, func() {
		accumulator := &Accumulator{Source: &Square{}}

		Convey("it accumulates what the Source emits, not the carrier", func() {
			So(accumulator.Step(types.Scalar(2)), ShouldEqual, types.Scalar(4))
			So(accumulator.Step(types.Scalar(3)), ShouldEqual, types.Scalar(13))
		})
	})

	Convey("Given an Accumulator that has never stepped", t, func() {
		accumulator := &Accumulator{}

		Convey("its total is the additive identity", func() {
			So(accumulator.Total(), ShouldEqual, types.Scalar(0))
		})
	})
}

func TestGate(t *testing.T) {
	Convey("Given a Gate with no When", t, func() {
		gate := &Gate{}

		Convey("nothing passes: an ungated Gate emits zero", func() {
			So(gate.Step(types.Scalar(5)), ShouldEqual, types.Scalar(0))
		})
	})

	Convey("Given a Gate held open", t, func() {
		gate := &Gate{When: &Constant{Value: 1}}

		Convey("an omitted Source emits the carrier", func() {
			So(gate.Step(types.Scalar(5)), ShouldEqual, types.Scalar(5))
		})
	})

	Convey("Given a Gate held closed", t, func() {
		gate := &Gate{Source: &Constant{Value: 9}, When: &Constant{Value: 0}}

		Convey("the Source is not emitted", func() {
			So(gate.Step(types.Scalar(5)), ShouldEqual, types.Scalar(0))
		})
	})

	Convey("Given a Gate over a Source", t, func() {
		gate := &Gate{Source: &Constant{Value: 9}, When: &Constant{Value: 1}}

		Convey("the Source is emitted, not the carrier", func() {
			So(gate.Step(types.Scalar(5)), ShouldEqual, types.Scalar(9))
		})
	})
}
