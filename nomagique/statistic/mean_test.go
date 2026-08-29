package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestMean(t *testing.T) {
	Convey("Given a populated generic sample collection", t, func() {
		output := types.Frame{}
		output.Put(types.MustSampleSymbol(0), 2)
		output.Put(types.MustSampleSymbol(1), 4)
		output.Put(types.MustSampleSymbol(2), 9)

		Mean(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolMean), ShouldEqual, 5.0)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given an empty sample collection", t, func() {
		output := types.Frame{}
		Mean(&output)

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
		output := input
		Mean(&output)
	}
}
