package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestMedianAbsolute(t *testing.T) {
	Convey("Given a populated generic sample collection", t, func() {
		output := types.Frame{}
		output.Put(types.MustSampleSymbol(0), -2)
		output.Put(types.MustSampleSymbol(1), -4)
		output.Put(types.MustSampleSymbol(2), 9)

		MedianAbsolute(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 4.0)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
		So(output.MustGet(SymbolCount), ShouldEqual, 3.0)
	})

	Convey("Given an even populated sample collection", t, func() {
		output := types.Frame{}
		output.Put(types.MustSampleSymbol(0), -8)
		output.Put(types.MustSampleSymbol(1), 1)
		output.Put(types.MustSampleSymbol(2), 3)
		output.Put(types.MustSampleSymbol(3), -6)

		MedianAbsolute(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 4.5)
		So(output.MustGet(SymbolReady), ShouldEqual, 1.0)
	})

	Convey("Given an empty sample collection", t, func() {
		output := types.Frame{}
		MedianAbsolute(&output)

		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolReady), ShouldEqual, 0.0)
	})

	Convey("Given a non-finite sample", t, func() {
		output := types.Frame{}
		output.Put(types.MustSampleSymbol(0), 1)
		output.Put(types.MustSampleSymbol(1), math.Inf(1))

		MedianAbsolute(&output)

		So(output.Err, ShouldNotBeNil)
	})
}

func TestMedianAbsoluteOf(t *testing.T) {
	Convey("Given a finite slice", t, func() {
		value, ok := MedianAbsoluteOf([]float64{-2, -4, 9})

		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 4.0)
	})

	Convey("Given an empty slice", t, func() {
		value, ok := MedianAbsoluteOf(nil)

		So(ok, ShouldBeFalse)
		So(value, ShouldEqual, 0.0)
	})

	Convey("Given a slice with a non-finite value", t, func() {
		value, ok := MedianAbsoluteOf([]float64{1, math.Inf(1)})

		So(ok, ShouldBeFalse)
		So(value, ShouldEqual, 0.0)
	})
}

func BenchmarkMedianAbsolute(benchmark *testing.B) {
	input := types.Frame{}

	for index := range types.MaxSamples {
		input.Put(types.MustSampleSymbol(index), float64(index)-64)
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		output := input
		MedianAbsolute(&output)
	}
}
