package leadlag

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

func TestLeadLagNumber(t *testing.T) {
	Convey("Given an anchor and follower price pair", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should compose pairwise inefficiency and significance", func() {
			input := nomagique.Frame{}
			input.Put(nomagique.SampleValue, 110.0)
			input.Put(calculus.SymbolLeft, 100.0)
			input.Put(calculus.SymbolRight, 110.0)
			input.Put(calculus.SymbolValue, 110.0)
			input.Put(calculus.SymbolScale, 1.0)
			input.Put(nmtypes.EventTimeSec, float64(1_700_007_000))
			input.Put(nmtypes.EventTimeNsec, 0)
			input.Put(statistic.SymbolDispersionHalflife, 30.0)

			output, err := signal.number(
				[2]string{"AAA/USD", "BBB/USD"},
				input,
			)

			So(err, ShouldBeNil)
			So(output.MustGet(SymbolLagRatio), ShouldBeGreaterThan, 0)
		})
	})
}
