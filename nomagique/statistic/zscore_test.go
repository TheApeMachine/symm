package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func zscoreObservationForTest(value float64, sec float64, halflife float64) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)
	input.Put(SymbolDispersionHalflife, halflife)

	return input
}

func TestZScore(t *testing.T) {
	Convey("Given a state without a composed baseline", t, func() {
		stream := types.NewStream(ZScore(""), types.Frame{})

		Convey("It should report not ready without failing", func() {
			output := stream.Step(zscoreObservationForTest(100, 1000, 5))
			So(output.Err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)
		})
	})

	Convey("Given a baseline of one hundred in state", t, func() {
		state := types.Frame{}
		state.Put(SymbolBaselineValue, 100)
		stream := types.NewStream(ZScore(""), state)

		Convey("The first residual should seed the dispersion at unit score", func() {
			output := stream.Step(zscoreObservationForTest(110, 1000, 5))
			So(output.Err, ShouldBeNil)

			residual, _ := output.Get(SymbolResidual)
			score, _ := output.Get(SymbolZScore)

			So(residual, ShouldEqual, 10)
			So(score, ShouldEqual, 1)
		})

		Convey("A resting value should score zero", func() {
			first := stream.Step(zscoreObservationForTest(110, 1000, 5))
			So(first.Err, ShouldBeNil)

			output := stream.Step(zscoreObservationForTest(100, 1001, 5))
			So(output.Err, ShouldBeNil)

			score, _ := output.Get(SymbolZScore)
			So(score, ShouldEqual, 0)
		})
	})

	Convey("Given an observation with missing inputs or regressed time", t, func() {
		Convey("It should fail the transition", func() {
			failed := types.Frame{}
			ZScore("")(&failed)
			So(failed.Err, ShouldNotBeNil)

			failed = zscoreObservationForTest(100, 1000, -5)
			ZScore("")(&failed)
			So(failed.Err, ShouldNotBeNil)
		})

		Convey("Omitting halflife should self-adapt gracefully", func() {
			state := types.Frame{}
			state.Put(SymbolBaselineValue, 100)
			state.Put(SymbolBaselineSpan, 10)
			stream := types.NewStream(ZScore(""), state)

			input := types.Frame{}
			input.Put(types.SampleValue, 110)
			input.Put(SymbolUnixSec, 1000)
			input.Put(SymbolUnixNsec, 0)

			output := stream.Step(input)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolZScore), ShouldEqual, 1)
		})
	})
}

func BenchmarkZScore(b *testing.B) {
	state := types.Frame{}
	state.Put(SymbolBaselineValue, 100)
	stream := types.NewStream(ZScore(""), state)
	input := zscoreObservationForTest(100, 1000, 5)
	b.ReportAllocs()

	for b.Loop() {
		_ = stream.Step(input)
	}
}
