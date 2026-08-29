package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestStability(t *testing.T) {
	Convey("Given a sample collection centered on its mean", t, func() {
		output := stabilityRing(10, 50, 90)
		output.Put(SymbolMean, 50)

		Stability("")(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolRange), ShouldEqual, 80.0)
		So(output.MustGet(SymbolStability), ShouldEqual, 0.5)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given a collapsed sample range", t, func() {
		output := stabilityRing(100, 100)
		output.Put(SymbolMean, 100)

		Stability("")(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolStability), ShouldEqual, 1.0)
	})

	Convey("Given too little evidence", t, func() {
		output := stabilityRing(100)

		Stability("")(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})
}

/*
stabilityRing writes one retained ring of the default series with the given
values, exactly as a composed window would retain them.
*/
func stabilityRing(values ...float64) types.Frame {
	input := types.Frame{}

	for index, value := range values {
		input.Put(types.MustSampleSymbol(index), value)
	}

	input.Put(types.SampleCount, float64(len(values)))
	input.Put(types.SampleHead, 0)
	input.Put(temporal.SymbolCapacity, float64(len(values)))

	return input
}

func BenchmarkStability(benchmark *testing.B) {
	input := types.Frame{}
	input.Put(SymbolMean, float64(types.MaxSamples-1)/2)
	input.Put(types.SampleCount, float64(types.MaxSamples))
	input.Put(types.SampleHead, 0)
	input.Put(temporal.SymbolCapacity, float64(types.MaxSamples))

	for index := range types.MaxSamples {
		input.Put(types.MustSampleSymbol(index), float64(index))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		output := input
		Stability("")(&output)
	}
}
