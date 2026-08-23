package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestLogChange(t *testing.T) {
	Convey("Given consecutive positive observations", t, func() {
		stream := types.NewStream(
			LogChange(types.SampleValue),
			types.Frame{},
		)
		first, err := stream.Step(equationSample(100, 1))
		So(err, ShouldBeNil)
		So(first.Has(SymbolChange), ShouldBeFalse)
		second, err := stream.Step(equationSample(110, 2))
		So(err, ShouldBeNil)
		So(second.MustGet(SymbolChange), ShouldEqual, math.Log(1.1))
	})

	Convey("Given regressing event time", t, func() {
		stream := types.NewStream(
			LogChange(types.SampleValue),
			types.Frame{},
		)
		_, err := stream.Step(equationSample(100, 2))
		So(err, ShouldBeNil)
		_, err = stream.Step(equationSample(110, 1))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "temporal: observer event time must not regress")
	})
}

func BenchmarkLogChange(benchmark *testing.B) {
	stream := types.NewStream(
		LogChange(types.SampleValue),
		types.Frame{},
	)
	_, _ = stream.Step(equationSample(100, 0))
	input := equationSample(110, 1)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = stream.Step(input)
	}
}
