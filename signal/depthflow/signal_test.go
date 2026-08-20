package depthflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestDepthFlowNumber(t *testing.T) {
	Convey("Given touch and deep book quantities on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		go signal.Run()

		Convey("It should compose the touch/deep imbalance family", func() {
			touch := 2.0
			deep := 8.0

			input := nomagique.Frame{}
			input.Put(calculus.SymbolLeft, touch)
			input.Put(calculus.SymbolRight, deep)
			input.Put(calculus.SymbolLevel, deep)
			input.Put(calculus.SymbolClock, touch/(touch+deep))
			input.Put(nomagique.SampleValue, touch+deep)
			input.Put(statistic.SymbolBaseline, touch+deep)
			input.Put(calculus.SymbolValue, touch)
			input.Put(calculus.SymbolScale, touch+deep)
			input.Put(nmtypes.EventTimeSec, float64(1_700_001_000))
			input.Put(nmtypes.EventTimeNsec, 0)
			input.Put(statistic.SymbolDispersionHalflife, 30.0)

			output, err := signal.number("AAA/USD", input)

			So(err, ShouldBeNil)
			So(output.MustGet(SymbolTouchImbalance), ShouldNotEqual, 0)
			So(output.MustGet(SymbolDeepImbalance), ShouldNotEqual, 0)
		})
	})
}
