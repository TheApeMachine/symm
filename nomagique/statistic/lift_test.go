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
			output := liftObservationForTest(100, 100)
			Lift(&output)
			So(output.Err, ShouldBeNil)

			result, _ := output.Get(SymbolResult)
			So(result, ShouldEqual, 0)
		})
	})

	Convey("Given a value lifted above its baseline", t, func() {
		Convey("It should report the lift as a fraction of the baseline", func() {
			output := liftObservationForTest(150, 100)
			Lift(&output)
			So(output.Err, ShouldBeNil)

			result, _ := output.Get(SymbolResult)
			So(result, ShouldEqual, 0.5)
		})
	})

	Convey("Given a non-positive baseline or missing inputs", t, func() {
		Convey("It should report not ready with a zero result", func() {
			output := liftObservationForTest(100, 0)
			Lift(&output)
			So(output.Err, ShouldBeNil)
			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)
			result, _ := output.Get(SymbolResult)
			So(result, ShouldEqual, 0)
			failed := types.Frame{}
			Lift(&failed)
			So(failed.Err, ShouldNotBeNil)
		})
	})
}

func BenchmarkLift(b *testing.B) {
	input := liftObservationForTest(100, 100)
	b.ReportAllocs()

	for b.Loop() {
		output := input
		Lift(&output)
	}
}
