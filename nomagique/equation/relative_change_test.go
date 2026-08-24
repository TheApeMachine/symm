package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestRelativeChange(t *testing.T) {
	Convey("Given a positive series that contracts", t, func() {
		stream := types.NewStream(
			RelativeChange(types.SampleValue),
			types.Frame{},
		)
		first := stream.Step(equationSample(100, 1))
		So(first.Err, ShouldBeNil)
		So(first.Has(SymbolRelativeChange), ShouldBeFalse)

		second := stream.Step(equationSample(80, 2))
		So(second.Err, ShouldBeNil)
		So(second.MustGet(SymbolChange), ShouldEqual, -20.0)
		So(second.MustGet(SymbolRelativeChange), ShouldEqual, -0.2)
		So(second.MustGet(statistic.SymbolMaturity), ShouldEqual, 2.0/3.0)
	})

	Convey("Given a zero prior denominator", t, func() {
		stream := types.NewStream(
			RelativeChange(types.SampleValue),
			types.Frame{},
		)
		first := stream.Step(equationSample(0, 1))
		So(first.Err, ShouldBeNil)
		So(first.Has(SymbolRelativeChange), ShouldBeFalse)

		second := stream.Step(equationSample(1, 2))
		So(second.Err, ShouldBeNil)
		So(second.Has(SymbolRelativeChange), ShouldBeFalse)
		So(second.MustGet(SymbolChange), ShouldEqual, 1.0)
	})
}

func BenchmarkRelativeChange(benchmark *testing.B) {
	stream := types.NewStream(
		RelativeChange(types.SampleValue),
		types.Frame{},
	)
	_ = stream.Step(equationSample(100, 0))
	input := equationSample(80, 1)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = stream.Step(input)
	}
}
