package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestAllocationCalculate(t *testing.T) {
	Convey("Given a decision that does not enter", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		decision := types.NewDecision(types.ActionNothing, "BTC/USD")
		symbol.Decisions.Store("BTC/USD", decision)
		thesis.Symbols.Store("BTC/USD", symbol)
		allocation := NewAllocation(t.Context(), nil)

		err := allocation.Calculate(thesis)

		Convey("It should leave the decision untouched without broker state", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func BenchmarkAllocationCalculate(b *testing.B) {
	thesis := types.NewThesis(b.Context(), nil)
	symbol := types.NewSymbol("BTC/USD", nil)
	symbol.Decisions.Store(
		"BTC/USD",
		types.NewDecision(types.ActionNothing, "BTC/USD"),
	)
	thesis.Symbols.Store("BTC/USD", symbol)
	allocation := NewAllocation(b.Context(), nil)

	for b.Loop() {
		if err := allocation.Calculate(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
