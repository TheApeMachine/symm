package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func baselineObservationForTest(
	value float64,
	sec float64,
	fastHalflife float64,
	slowHalflife float64,
) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(temporal.SymbolCapacity, 8)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)
	input.Put(SymbolBaselineFastHalflife, fastHalflife)
	input.Put(SymbolBaselineSlowHalflife, slowHalflife)
	input.Put(SymbolDispersionHalflife, 5)

	return input
}

func baselineMachine() types.Primitive {
	return types.Pipe(
		temporal.Window,
		types.Fork(Baseline, ZScore),
	)
}

func TestBaseline(t *testing.T) {
	Convey("Given a composed window and baseline receiving its first value", t, func() {
		stream := types.NewStream(baselineMachine(), types.Frame{})

		Convey("It should seed the baseline with the value itself", func() {
			output, err := stream.Step(baselineObservationForTest(100, 1000, 2, 60))
			So(err, ShouldBeNil)

			baseline, found := output.Get(SymbolBaselineValue)
			So(found, ShouldBeTrue)
			So(baseline, ShouldEqual, 100)

			window, found := output.Get(SymbolBaselineWindow)
			So(found, ShouldBeTrue)
			So(window, ShouldEqual, 1)
		})
	})

	Convey("Given a direct, low-noise ramp", t, func() {
		stream := types.NewStream(baselineMachine(), types.Frame{})

		for index := range 8 {
			_, err := stream.Step(baselineObservationForTest(
				100+float64(index), 1000+float64(index), 2, 60,
			))
			So(err, ShouldBeNil)
		}

		ramp, err := stream.Step(baselineObservationForTest(108, 1008, 2, 60))
		So(err, ShouldBeNil)

		Convey("It should demand a sharp baseline and track the ramp", func() {
			efficiency, _ := ramp.Get(SymbolBaselineEfficiency)
			So(efficiency, ShouldEqual, 1)

			window, _ := ramp.Get(SymbolBaselineWindow)
			So(window, ShouldBeLessThan, 10)
		})
	})

	Convey("Given a choppy, directionless series", t, func() {
		stream := types.NewStream(baselineMachine(), types.Frame{})

		for index := range 8 {
			chop := 100.0

			if index%2 == 1 {
				chop = 103
			}

			_, err := stream.Step(baselineObservationForTest(chop, 1000+float64(index), 2, 60))
			So(err, ShouldBeNil)
		}

		choppy, err := stream.Step(baselineObservationForTest(103, 1008, 2, 60))
		So(err, ShouldBeNil)

		Convey("It should force the effective window wide and hold the baseline", func() {
			efficiency, _ := choppy.Get(SymbolBaselineEfficiency)
			So(efficiency, ShouldBeLessThan, 0.5)

			window, _ := choppy.Get(SymbolBaselineWindow)
			So(window, ShouldBeGreaterThan, 10)

			baseline, _ := choppy.Get(SymbolBaselineValue)
			So(baseline, ShouldBeBetween, 99, 104)
		})
	})

	Convey("Given a calm series followed by a sudden departure", t, func() {
		stream := types.NewStream(baselineMachine(), types.Frame{})

		for index := range 8 {
			_, err := stream.Step(baselineObservationForTest(100, 1000+float64(index), 2, 60))
			So(err, ShouldBeNil)
		}

		calm, err := stream.Step(baselineObservationForTest(100, 1008, 2, 60))
		So(err, ShouldBeNil)

		spiked, err := stream.Step(baselineObservationForTest(115, 1009, 2, 60))
		So(err, ShouldBeNil)

		Convey("The composed z-score should spike before the baseline moves", func() {
			calmScore, _ := calm.Get(SymbolZScore)
			spikedScore, _ := spiked.Get(SymbolZScore)

			So(calmScore, ShouldEqual, 0)
			So(spikedScore, ShouldBeGreaterThan, 1)

			baseline, _ := spiked.Get(SymbolBaselineValue)
			So(baseline, ShouldBeLessThan, 105)
		})
	})

	Convey("Given an observation with regressed event time", t, func() {
		stream := types.NewStream(baselineMachine(), types.Frame{})

		_, err := stream.Step(baselineObservationForTest(100, 1000, 2, 60))
		So(err, ShouldBeNil)

		Convey("It should fail the transition", func() {
			_, err := stream.Step(baselineObservationForTest(101, 999, 2, 60))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkBaseline(b *testing.B) {
	stream := types.NewStream(baselineMachine(), types.Frame{})
	input := baselineObservationForTest(100, 1000, 2, 60)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
