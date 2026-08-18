package statistic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
	. "github.com/smartystreets/goconvey/convey"
)

func velocityObservationForTest(value float64, sec float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)

	return input
}

func TestVelocity(t *testing.T) {
	Convey("Given a single observation", t, func() {
		stream := nomagique.NewStream(Velocity, nomagique.Frame{})

		Convey("It should seed the differencer without a delta", func() {
			output, err := stream.Step(velocityObservationForTest(100, 1000))
			So(err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)

			_, hasDelta := output.Get(SymbolVelocityDelta)
			So(hasDelta, ShouldBeFalse)
		})
	})

	Convey("Given a rising then accelerating series", t, func() {
		stream := nomagique.NewStream(Velocity, nomagique.Frame{})

		_, err := stream.Step(velocityObservationForTest(100, 1000))
		So(err, ShouldBeNil)

		steady, err := stream.Step(velocityObservationForTest(102, 1001))
		So(err, ShouldBeNil)

		accelerating, err := stream.Step(velocityObservationForTest(106, 1002))
		So(err, ShouldBeNil)

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
		stream := nomagique.NewStream(Velocity, nomagique.Frame{})

		_, err := stream.Step(velocityObservationForTest(100, 1000))
		So(err, ShouldBeNil)

		Convey("It should fail the transition", func() {
			_, err := stream.Step(velocityObservationForTest(101, 999))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkVelocity(b *testing.B) {
	stream := nomagique.NewStream(Velocity, nomagique.Frame{})
	input := velocityObservationForTest(100, 1000)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
