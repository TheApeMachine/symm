package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestAdaptiveBaseline(t *testing.T) {
	Convey("Given successive observations", t, func() {
		baseline := nomagique.NewStream(AdaptiveBaseline(), nomagique.Frame{})

		first, err := baseline.Step(baselineObservation(100, 1))
		So(err, ShouldBeNil)
		second, err := baseline.Step(baselineObservation(100, 2))
		So(err, ShouldBeNil)

		Convey("It should compose a mean, stability, and next span", func() {
			So(first.MustGet(statistic.SymbolMean), ShouldEqual, 100.0)
			So(first.MustGet(nmtypes.Span), ShouldEqual, 2.0)
			So(second.MustGet(statistic.SymbolMean), ShouldEqual, 100.0)
			So(second.MustGet(statistic.SymbolStability), ShouldEqual, 1.0)
			So(second.MustGet(temporal.SymbolCapacity), ShouldEqual, 2.0)
		})
	})
}

func baselineObservation(value float64, seconds float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nomagique.SampleValue, value)
	input.Put(temporal.SymbolUnixSec, seconds)
	input.Put(temporal.SymbolUnixNsec, 0)

	return input
}

func BenchmarkAdaptiveBaseline(benchmark *testing.B) {
	baseline := nomagique.NewStream(AdaptiveBaseline(), nomagique.Frame{})
	input := baselineObservation(100, 1)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = baseline.Step(input)
	}
}
