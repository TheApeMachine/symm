package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

type stubHawkes struct {
	symbols  []string
	outcomes map[string]excitation.Outcome
}

func (source stubHawkes) Symbols() []string { return source.symbols }

func (source stubHawkes) Outcome(symbol string) (excitation.Outcome, bool) {
	outcome, found := source.outcomes[symbol]

	return outcome, found
}

func TestAnalyzerInterest(t *testing.T) {
	Convey("Given open inventory and stronger flat Hawkes leaders", t, func() {
		weak := excitation.Outcome{Readiness: excitation.Readiness{Intensity: true}}
		weak.BuyArrivalRate = 1
		strong := excitation.Outcome{Readiness: excitation.Readiness{Intensity: true}}
		strong.BuyArrivalRate = 9
		mid := excitation.Outcome{Readiness: excitation.Readiness{Intensity: true}}
		mid.BuyArrivalRate = 4

		analyzer := &Analyzer{
			hawkes: stubHawkes{
				symbols: []string{"AAA/USD", "ONDO/USD", "ZZZ/USD"},
				outcomes: map[string]excitation.Outcome{
					"AAA/USD":  strong,
					"ONDO/USD": weak,
					"ZZZ/USD":  mid,
				},
			},
		}
		thesis := types.NewThesis(nil, nil)
		thesis.Holdings.Store("ONDO/USD", &types.Holding{
			Symbol: "ONDO/USD",
			Status: types.OPEN,
		})

		interest := analyzer.Interest(thesis)

		Convey("It should pin the open lot then fill by intensity", func() {
			So(interest, ShouldResemble, []string{"ONDO/USD", "AAA/USD", "ZZZ/USD"})
		})
	})
}
