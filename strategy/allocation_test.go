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

	Convey("Given an enter decision without a broker desk", t, func() {
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		allocation := NewAllocation(t.Context(), nil)

		err := allocation.Calculate([]*types.Decision{decision})

		Convey("It should report that a broker desk is required", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "broker desk required")
		})
	})
}
