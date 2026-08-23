package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestStability(t *testing.T) {
	Convey("Given a sample collection centered on its mean", t, func() {
		input := types.Frame{}
		input.Put(nomagique.MustSampleSymbol(0), 10)
		input.Put(nomagique.MustSampleSymbol(1), 50)
		input.Put(nomagique.MustSampleSymbol(2), 90)
		input.Put(SymbolMean, 50)

		_, output, err := Stability(types.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolRange), ShouldEqual, 80.0)
		So(output.MustGet(SymbolStability), ShouldEqual, 0.5)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given a collapsed sample range", t, func() {
		input := types.Frame{}
		input.Put(nomagique.MustSampleSymbol(0), 100)
		input.Put(nomagique.MustSampleSymbol(1), 100)
		input.Put(SymbolMean, 100)

		_, output, err := Stability(types.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolStability), ShouldEqual, 1.0)
	})

	Convey("Given too little evidence", t, func() {
		input := types.Frame{}
		input.Put(nomagique.MustSampleSymbol(0), 100)

		_, output, err := Stability(types.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})
}

func BenchmarkStability(benchmark *testing.B) {
	input := types.Frame{}
	input.Put(SymbolMean, float64(nomagique.MaxSamples-1)/2)

	for index := range nomagique.MaxSamples {
		input.Put(nomagique.MustSampleSymbol(index), float64(index))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = Stability(types.Frame{}, input)
	}
}
