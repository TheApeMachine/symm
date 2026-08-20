package correlation

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestCorrelationNumber(t *testing.T) {
	Convey("Given tickers across two symbols", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		start := time.Unix(1_700_000_000, 0).UTC()

		Convey("It should compose conditioned correlation evidence per tick", func() {
			for index := 0; index < 4; index++ {
				input := nomagique.Frame{}
				input.Put(calculus.SymbolCurrent, 100+float64(index)*10)
				input.Put(calculus.SymbolPrevious, 90+float64(index)*10)
				input.Put(calculus.SymbolLeft, 100+float64(index)*10)
				input.Put(calculus.SymbolRight, 200+float64(index)*10)
				input.Put(calculus.SymbolScale, 1.0)
				input.Put(nomagique.SampleValue, 100+float64(index)*10)
				input.Put(nmtypes.EventTimeSec, float64(start.Add(time.Duration(index)*time.Second).Unix()))
				input.Put(nmtypes.EventTimeNsec, 0)
				input.Put(statistic.SymbolDispersionHalflife, 30.0)

				output, err := signal.number(
					[2]string{"AAA/USD", "BBB/USD"},
					input,
				)

				So(err, ShouldBeNil)
				So(output.MustGet(SymbolCohortRelation), ShouldBeGreaterThan, 0)
			}
		})
	})
}
