package statistic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
	. "github.com/smartystreets/goconvey/convey"
)

func zscoreObservationForTest(value float64, sec float64, halflife float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(SymbolUnixSec, sec)
	input.Put(SymbolUnixNsec, 0)
	input.Put(SymbolDispersionHalflife, halflife)

	return input
}

func TestZScore(t *testing.T) {
	Convey("Given a state without a composed baseline", t, func() {
		stream := nomagique.NewStream(ZScore, nomagique.Frame{})

		Convey("It should report not ready without failing", func() {
			output, err := stream.Step(zscoreObservationForTest(100, 1000, 5))
			So(err, ShouldBeNil)

			ready, _ := output.Get(SymbolReady)
			So(ready, ShouldEqual, 0)
		})
	})

	Convey("Given a baseline of one hundred in state", t, func() {
		state := nomagique.Frame{}
		state.Put(SymbolBaselineValue, 100)
		stream := nomagique.NewStream(ZScore, state)

		Convey("The first residual should seed the dispersion at unit score", func() {
			output, err := stream.Step(zscoreObservationForTest(110, 1000, 5))
			So(err, ShouldBeNil)

			residual, _ := output.Get(SymbolResidual)
			score, _ := output.Get(SymbolZScore)

			So(residual, ShouldEqual, 10)
			So(score, ShouldEqual, 1)
		})

		Convey("A resting value should score zero", func() {
			_, err := stream.Step(zscoreObservationForTest(110, 1000, 5))
			So(err, ShouldBeNil)

			output, err := stream.Step(zscoreObservationForTest(100, 1001, 5))
			So(err, ShouldBeNil)

			score, _ := output.Get(SymbolZScore)
			So(score, ShouldEqual, 0)
		})
	})

	Convey("Given an observation with missing inputs or regressed time", t, func() {
		Convey("It should fail the transition", func() {
			_, _, err := ZScore(nomagique.Frame{}, nomagique.Frame{})
			So(err, ShouldNotBeNil)

			_, _, err = ZScore(nomagique.Frame{}, zscoreObservationForTest(100, 1000, 0))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkZScore(b *testing.B) {
	state := nomagique.Frame{}
	state.Put(SymbolBaselineValue, 100)
	stream := nomagique.NewStream(ZScore, state)
	input := zscoreObservationForTest(100, 1000, 5)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
