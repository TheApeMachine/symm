package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestNormalize(t *testing.T) {
	Convey("Given a causal positive series", t, func() {
		stream := nomagique.NewStream(Normalize(), nomagique.Frame{})
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
}

func BenchmarkNormalize(benchmark *testing.B) {
	stream := nomagique.NewStream(Normalize(), nomagique.Frame{})
	input := equationSample(20, 1)
	_, _ = stream.Step(equationSample(10, 0))
	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for range benchmark.N {
		_, _ = stream.Step(input)
	}
}

func equationSample(value float64, seconds float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(temporal.SymbolUnixSec, seconds)
	input.Put(temporal.SymbolUnixNsec, 0)

	return input
}
