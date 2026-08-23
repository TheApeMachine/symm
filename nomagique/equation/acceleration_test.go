package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestAcceleration(t *testing.T) {
	Convey("Given a quantity-clocked positive series", t, func() {
		stream := nomagique.NewStream(Acceleration(), types.Frame{})
		first, err := stream.Step(accelerationInput(2, 100, 0))
		So(err, ShouldBeNil)
		So(first.MustGet(SymbolClosed), ShouldEqual, 0.0)
		So(first.MustGet(SymbolTarget), ShouldEqual, 2.0)

		second, err := stream.Step(accelerationInput(2, 100, 1))
		So(err, ShouldBeNil)
		So(second.MustGet(SymbolClosed), ShouldEqual, 1.0)
		So(second.MustGet(calculus.SymbolRate), ShouldEqual, 4.0)
		So(second.Has(SymbolChange), ShouldBeFalse)
		So(second.MustGet(temporal.SymbolObservedSec), ShouldEqual, 0.0)
		So(second.MustGet(statistic.SymbolMaturity), ShouldEqual, 0.5)

		third, err := stream.Step(accelerationInput(2, 110, 2))
		So(err, ShouldBeNil)

		Convey("It closes the next empirical span with an exact log change", func() {
			So(third.MustGet(SymbolClosed), ShouldEqual, 1.0)
			So(third.MustGet(calculus.SymbolRate), ShouldEqual, 2.0)
			So(third.MustGet(SymbolChange), ShouldEqual, math.Log(1.1))
			So(third.MustGet(temporal.SymbolObservedSec), ShouldEqual, 1.0)
			So(third.MustGet(statistic.SymbolMaturity), ShouldEqual, 2.0/3.0)
		})
	})
}

func BenchmarkAcceleration(benchmark *testing.B) {
	stream := nomagique.NewStream(Acceleration(), types.Frame{})
	input := accelerationInput(2, 100, 1)
	_, _ = stream.Step(accelerationInput(2, 100, 0))
	benchmark.ReportAllocs()

	for range benchmark.N {
		_, _ = stream.Step(input)
	}
}

func accelerationInput(quantity float64, priceValue float64, seconds float64) types.Frame {
	input := types.Frame{}
	input.Put(nmtypes.Quantity, quantity)
	input.Put(nmtypes.AlphaPrice, priceValue)
	input.Put(temporal.SymbolUnixSec, seconds)
	input.Put(temporal.SymbolUnixNsec, 0)

	return input
}
