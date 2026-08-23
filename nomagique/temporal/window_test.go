package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func windowObservationForTest(value float64, capacity int) types.Frame {
	input := types.Frame{}
	input.Put(nmtypes.SampleValue, value)
	input.Put(SymbolUnixSec, float64(capacity*1000))
	input.Put(SymbolUnixNsec, 0)
	input.Put(nmtypes.Span, float64(capacity))

	return input
}

func TestWindow(t *testing.T) {
	Convey("Given successive values inside a window's capacity", t, func() {
		stream := nmtypes.NewStream(Window, types.Frame{})

		Convey("It should retain every value and echo the observation", func() {
			_, err := stream.Step(windowObservationForTest(100, 4))
			So(err, ShouldBeNil)

			output, err := stream.Step(windowObservationForTest(101, 4))
			So(err, ShouldBeNil)

			count, found := output.Get(nmtypes.SampleCount)
			So(found, ShouldBeTrue)
			So(count, ShouldEqual, 2)

			value, found := output.Get(nmtypes.SampleValue)
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 101)
		})
	})

	Convey("Given more values than the window's capacity", t, func() {
		stream := nmtypes.NewStream(Window, types.Frame{})

		for value := 100.0; value < 104; value++ {
			_, err := stream.Step(windowObservationForTest(value, 3))
			So(err, ShouldBeNil)
		}

		Convey("It should evict the oldest and retain the newest", func() {
			state := stream.Project()

			count, _ := state.Get(nmtypes.SampleCount)
			So(count, ShouldEqual, 3)

			newest, _ := state.Get(nmtypes.MustSampleSymbol(0))
			So(newest, ShouldEqual, 103)

			oldest, _ := state.Get(nmtypes.MustSampleSymbol(1))
			So(oldest, ShouldEqual, 101)
		})
	})

	Convey("Given an observation without a value or event time", t, func() {
		Convey("It should fail the transition", func() {
			_, _, err := Window(types.Frame{}, types.Frame{})
			So(err, ShouldNotBeNil)

			_, _, err = Window(types.Frame{}, windowObservationForTest(100, 0))
			So(err, ShouldNotBeNil)

			_, _, err = Window(types.Frame{}, windowObservationForTest(100, nmtypes.MaxSamples+1))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkWindow(b *testing.B) {
	stream := nmtypes.NewStream(Window, types.Frame{})
	input := windowObservationForTest(100, 128)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
