package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestAllocationCalculate(t *testing.T) {
	Convey("Given a decision that does not enter", t, func() {
		decision := types.NewDecision(types.ActionNothing, "BTC/USD")
		allocation := NewAllocation(t.Context(), nil)

		err := allocation.Calculate([]*types.Decision{decision})

		Convey("It should leave the decision untouched without broker state", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func BenchmarkAllocationCalculate(b *testing.B) {
	decisions := []*types.Decision{
		types.NewDecision(types.ActionNothing, "BTC/USD"),
	}
	allocation := NewAllocation(b.Context(), nil)

	for b.Loop() {
		if err := allocation.Calculate(decisions); err != nil {
			b.Fatal(err)
		}
	}
}
