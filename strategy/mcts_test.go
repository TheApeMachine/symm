package strategy

import (
	"math"
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
		state := StrategyState{Treatment: 0.4, GraphReward: 0.25}

		Convey("It should model standing aside as do(0)", func() {
			next := state.ApplyAction(ActionNothing).(StrategyState)

			So(next.IsTerminal(), ShouldBeTrue)
			So(next.Treatment, ShouldEqual, 0)
			So(next.Reward, ShouldEqual, 0)
			So(next.GetInterventionLevel(ActionNothing), ShouldEqual, 0)
		})

		Convey("It should retain the observed treatment for Enter", func() {
			next := state.ApplyAction(ActionEnter).(StrategyState)

			So(next.IsTerminal(), ShouldBeTrue)
			So(next.Treatment, ShouldEqual, state.Treatment)
			So(next.Reward, ShouldEqual, state.GraphReward)
			So(next.GetInterventionLevel(ActionEnter), ShouldEqual, state.Treatment)
		})
	})
}

func TestStrategyStateGetPossibleActions(t *testing.T) {
	Convey("Given graph-adjusted admission state", t, func() {
		Convey("It should expose only standing aside when Enter is inadmissible", func() {
			state := StrategyState{CanEnter: false}

			So(state.GetPossibleActions(), ShouldResemble, []float64{ActionNothing})
		})

		Convey("It should expose both causal alternatives when Enter is admissible", func() {
			state := StrategyState{CanEnter: true}

			So(state.GetPossibleActions(), ShouldResemble, []float64{
				ActionNothing,
				ActionEnter,
			})
		})
	})
}

func strategyRowsFixture(treatmentDirection float64) [][]float64 {
	rows := make([][]float64, 12)

	for index := range rows {
		treatment := float64(index % 2)
		rows[index] = []float64{
			float64(index),
			math.Sin(float64(index)),
			treatment,
			math.Sin(float64(index)) +
				treatmentDirection*treatment/float64(len(rows)),
		}
	}

	return rows
}

func BenchmarkStrategyStateApplyAction(b *testing.B) {
	state := StrategyState{Treatment: 0.4, CanEnter: true}

	for b.Loop() {
		_ = state.ApplyAction(ActionEnter)
	}
}
