package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func velocityObservationForTest(value float64, sec float64) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)

	return input
}

func TestVelocity(t *testing.T) {
	Convey("Given a single observation", t, func() {
		stream := types.NewStream(Velocity(""), types.Frame{})

		Convey("It should seed the differencer without a delta", func() {
			output := stream.Step(velocityObservationForTest(100, 1000))
			So(output.Err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)

			_, hasDelta := output.Get(SymbolVelocityDelta)
			So(hasDelta, ShouldBeFalse)
		})
	})

	Convey("Given a rising then accelerating series", t, func() {
		stream := types.NewStream(Velocity(""), types.Frame{})

		first := stream.Step(velocityObservationForTest(100, 1000))
		So(first.Err, ShouldBeNil)

		steady := stream.Step(velocityObservationForTest(102, 1001))
		So(steady.Err, ShouldBeNil)

		accelerating := stream.Step(velocityObservationForTest(106, 1002))
		So(accelerating.Err, ShouldBeNil)

		Convey("It should report the raw delta, elapsed, then acceleration", func() {
			delta, _ := steady.Get(SymbolVelocityDelta)
			elapsed, _ := steady.Get(SymbolVelocityElapsed)
			So(delta, ShouldEqual, 2)
			So(elapsed, ShouldEqual, 1)

			_, hasAcceleration := steady.Get(SymbolVelocityAcceleration)
			So(hasAcceleration, ShouldBeFalse)

			acceleration, _ := accelerating.Get(SymbolVelocityAcceleration)
			So(acceleration, ShouldEqual, 2)
		})
	})

	Convey("Given an observation with regressed event time", t, func() {
		stream := types.NewStream(Velocity(""), types.Frame{})

		first := stream.Step(velocityObservationForTest(100, 1000))
		So(first.Err, ShouldBeNil)

		Convey("It should fail the transition", func() {
			failed := stream.Step(velocityObservationForTest(101, 999))
			So(failed.Err, ShouldNotBeNil)
		})
	})
}

func BenchmarkVelocity(b *testing.B) {
	stream := types.NewStream(Velocity(""), types.Frame{})
	input := velocityObservationForTest(100, 1000)
	b.ReportAllocs()

	for b.Loop() {
		_ = stream.Step(input)
	}
}
