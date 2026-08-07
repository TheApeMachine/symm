package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStrategyAction(t *testing.T) {
	Convey("Given the binary causal actions", t, func() {
		Convey("It should publish only Enter and Do Not Enter", func() {
			So(strategyAction(ActionNothing), ShouldEqual, types.ActionNothing)
			So(strategyAction(ActionEnter), ShouldEqual, types.ActionEnter)
		})

		Convey("It should reject an unknown action", func() {
			So(strategyAction(-1), ShouldEqual, types.Action(""))
		})
	})
}

func TestStrategyStateApplyAction(t *testing.T) {
	Convey("Given a causal state with an observed treatment", t, func() {
		state := StrategyState{Treatment: 0.4}

		Convey("It should model standing aside as do(0)", func() {
			next := state.ApplyAction(ActionNothing).(StrategyState)

			So(next.IsTerminal(), ShouldBeTrue)
			So(next.Treatment, ShouldEqual, 0)
			So(next.GetInterventionLevel(ActionNothing), ShouldEqual, 0)
		})

		Convey("It should retain the observed treatment for Enter", func() {
			next := state.ApplyAction(ActionEnter).(StrategyState)

			So(next.IsTerminal(), ShouldBeTrue)
			So(next.Treatment, ShouldEqual, state.Treatment)
			So(next.GetInterventionLevel(ActionEnter), ShouldEqual, state.Treatment)
		})
	})
}

func BenchmarkStrategyStateApplyAction(b *testing.B) {
	state := StrategyState{Treatment: 0.4}

	for b.Loop() {
		_ = state.ApplyAction(ActionEnter)
	}
}
