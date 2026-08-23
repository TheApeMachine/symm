package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func liftObservationForTest(value float64, baseline float64) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(SymbolBaseline, baseline)

	return input
}

func TestLift(t *testing.T) {
	Convey("Given a value resting exactly on its baseline", t, func() {
		Convey("It should report the honest zero", func() {
			_, output, err := Lift(
				types.Frame{},
				liftObservationForTest(100, 100),
			)
			So(err, ShouldBeNil)

			result, _ := output.Get(SymbolResult)
			So(result, ShouldEqual, 0)
		})
	})

	Convey("Given a value lifted above its baseline", t, func() {
		Convey("It should report the lift as a fraction of the baseline", func() {
			_, output, err := Lift(
				types.Frame{},
				liftObservationForTest(150, 100),
			)
			So(err, ShouldBeNil)

			result, _ := output.Get(SymbolResult)
			So(result, ShouldEqual, 0.5)
		})
	})

	Convey("Given a non-positive baseline or missing inputs", t, func() {
		Convey("It should report not ready or fail", func() {
			_, output, err := Lift(types.Frame{}, liftObservationForTest(100, 0))
			So(err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)

			_, _, err = Lift(types.Frame{}, types.Frame{})
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkLift(b *testing.B) {
	input := liftObservationForTest(100, 100)
	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = Lift(types.Frame{}, input)
	}
}
