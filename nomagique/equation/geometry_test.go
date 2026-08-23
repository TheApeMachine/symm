package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestGeometry(t *testing.T) {
	Convey("Given two ordered positive channels", t, func() {
		stream := nomagique.NewStream(Geometry(), types.Frame{})
		first, err := stream.Step(geometryInput(99, 101, 4, 1, 0))
		So(err, ShouldBeNil)
		So(first.MustGet(SymbolCenter), ShouldEqual, 100.0)
		So(first.MustGet(SymbolWidth), ShouldEqual, 2.0)
		So(first.MustGet(SymbolRelativeWidth), ShouldEqual, 0.02)
		So(first.MustGet(SymbolDissimilarity), ShouldEqual, 0.01)
		So(first.MustGet(SymbolBalance), ShouldEqual, math.Log(4))
		So(first.Has(SymbolCompression), ShouldBeFalse)

		second, err := stream.Step(geometryInput(99.5, 100.5, 2, 2, 1))
		So(err, ShouldBeNil)

		Convey("It measures compression against only the prior width", func() {
			So(second.MustGet(statistic.SymbolMean), ShouldEqual, 2.0)
			So(second.MustGet(SymbolWidth), ShouldEqual, 1.0)
			So(second.MustGet(SymbolDeviation), ShouldEqual, -1.0)
			So(second.MustGet(SymbolCompression), ShouldEqual, 0.5)
			So(second.MustGet(SymbolBalance), ShouldEqual, 0.0)
			So(second.MustGet(statistic.SymbolMaturity), ShouldEqual, 2.0/3.0)
		})
	})

	Convey("Given an interval wider than its center", t, func() {
		_, output, err := nomagique.Step(
			Geometry(),
			types.Frame{},
			geometryInput(1, 4, 1, 1, 0),
		)
		So(err, ShouldBeNil)

		Convey("It keeps relative width raw and bounds symmetric dissimilarity", func() {
			So(output.MustGet(SymbolRelativeWidth), ShouldEqual, 1.2)
			So(output.MustGet(SymbolDissimilarity), ShouldEqual, 0.6)
		})
	})

	Convey("Given a crossed or collapsed interval", t, func() {
		_, _, err := nomagique.Step(
			Geometry(),
			types.Frame{},
			geometryInput(101, 101, 1, 1, 0),
		)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "logic: positive order requires 0 < lower < upper")
	})
}

func BenchmarkGeometry(benchmark *testing.B) {
	stream := nomagique.NewStream(Geometry(), types.Frame{})
	input := geometryInput(99, 101, 4, 1, 1)
	benchmark.ReportAllocs()

	for range benchmark.N {
		_, _ = stream.Step(input)
	}
}

func geometryInput(
	alphaPrice float64,
	betaPrice float64,
	alphaQuantity float64,
	betaQuantity float64,
	seconds float64,
) types.Frame {
	input := equationSample(0, seconds)
	input.Put(nmtypes.AlphaPrice, alphaPrice)
	input.Put(nmtypes.BetaPrice, betaPrice)
	input.Put(nmtypes.AlphaQuantity, alphaQuantity)
	input.Put(nmtypes.BetaQuantity, betaQuantity)

	return input
}
