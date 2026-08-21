package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

func TestMean(t *testing.T) {
	Convey("Given a populated generic sample collection", t, func() {
		input := nomagique.Frame{}
		input.Put(nomagique.MustSampleSymbol(0), 2)
		input.Put(nomagique.MustSampleSymbol(1), 4)
		input.Put(nomagique.MustSampleSymbol(2), 9)

		_, output, err := Mean(nomagique.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolMean), ShouldEqual, 5.0)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given an empty sample collection", t, func() {
		_, output, err := Mean(nomagique.Frame{}, nomagique.Frame{})

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})
}

func BenchmarkMean(benchmark *testing.B) {
	input := nomagique.Frame{}

	for index := range nomagique.MaxSamples {
		input.Put(nomagique.MustSampleSymbol(index), float64(index))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = Mean(nomagique.Frame{}, input)
	}
}
