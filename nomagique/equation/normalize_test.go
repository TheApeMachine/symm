package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNormalize(t *testing.T) {
	Convey("Given a causal positive series", t, func() {
		stream := types.NewStream(Normalize(), types.Frame{})
		first, err := stream.Step(equationSample(10, 0))
		So(err, ShouldBeNil)
		So(first.Has(SymbolRatio), ShouldBeFalse)
		So(first.MustGet(statistic.SymbolMaturity), ShouldEqual, 0.5)

		second, err := stream.Step(equationSample(20, 1))
		So(err, ShouldBeNil)

		Convey("It scores the current value against the prior mean exactly", func() {
			So(second.MustGet(statistic.SymbolMean), ShouldEqual, 10.0)
			So(second.MustGet(SymbolRatio), ShouldEqual, 2.0)
			So(second.MustGet(SymbolLift), ShouldEqual, 1.0)
			So(second.MustGet(SymbolNormalized), ShouldEqual, 2.0/3.0)
			So(second.MustGet(statistic.SymbolMaturity), ShouldEqual, 2.0/3.0)
		})
	})

	Convey("Given a series whose baseline mean is zero", t, func() {
		stream := types.NewStream(Normalize(), types.Frame{})
		first, err := stream.Step(equationSample(0, 0))
		So(err, ShouldBeNil)
		So(first.Has(SymbolRatio), ShouldBeFalse)

		second, err := stream.Step(equationSample(0, 1))
		So(err, ShouldBeNil)

		Convey("It does not divide by zero and emits a zero ratio", func() {
			So(second.MustGet(statistic.SymbolMean), ShouldEqual, 0.0)
			So(second.MustGet(SymbolRatio), ShouldEqual, 0.0)
			So(second.MustGet(SymbolNormalized), ShouldEqual, 0.0)
			So(second.MustGet(statistic.SymbolMaturity), ShouldEqual, 2.0/3.0)
		})
	})
}

func BenchmarkNormalize(benchmark *testing.B) {
	stream := types.NewStream(Normalize(), types.Frame{})
	input := equationSample(20, 1)
	_, _ = stream.Step(equationSample(10, 0))
	benchmark.ReportAllocs()
	

	for benchmark.Loop() {
		_, _ = stream.Step(input)
	}
}

func equationSample(value float64, seconds float64) types.Frame {
	input := types.Frame{}
	input.Put(types.SampleValue, value)
	input.Put(temporal.SymbolUnixSec, seconds)
	input.Put(temporal.SymbolUnixNsec, 0)

	return input
}
