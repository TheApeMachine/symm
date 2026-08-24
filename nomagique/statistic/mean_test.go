package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestMean(t *testing.T) {
	Convey("Given a populated generic sample collection", t, func() {
		input := types.Frame{}
		input.Put(types.MustSampleSymbol(0), 2)
		input.Put(types.MustSampleSymbol(1), 4)
		input.Put(types.MustSampleSymbol(2), 9)

		output := Mean(input)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolMean), ShouldEqual, 5.0)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given an empty sample collection", t, func() {
		output := Mean(types.Frame{})

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})
}

func BenchmarkMean(benchmark *testing.B) {
	input := types.Frame{}

	for index := range types.MaxSamples {
		input.Put(types.MustSampleSymbol(index), float64(index))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = Mean(input)
	}
}
