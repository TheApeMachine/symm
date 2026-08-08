package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/mcts"
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

func TestStrategyStateSelectAction(t *testing.T) {
	Convey("Given weak causal uplift and graph evidence", t, func() {
		engine := strategyMCTSFixture()
		rows := strategyRowsFixture(1)
		state := StrategyState{Treatment: 1, CanEnter: true}

		Convey("It should enter when the causal intervention improves return", func() {
			action, err := state.SelectAction(engine, rows)

			So(err, ShouldBeNil)
			So(action, ShouldEqual, ActionEnter)
		})

		Convey("It should stand aside when contradiction outweighs the uplift", func() {
			reward, err := (graphEvidence{contradicts: 1}).Reward(rows, 3)
			So(err, ShouldBeNil)
			state.GraphReward = reward
			action, err := state.SelectAction(engine, rows)

			So(err, ShouldBeNil)
			So(action, ShouldEqual, ActionNothing)
		})

		Convey("It should admit support that outweighs weak negative uplift", func() {
			rows = strategyRowsFixture(-1)
			reward, err := (graphEvidence{supports: 1}).Reward(rows, 3)
			So(err, ShouldBeNil)
			state.GraphReward = reward
			action, err := state.SelectAction(engine, rows)

			So(err, ShouldBeNil)
			So(action, ShouldEqual, ActionEnter)
		})
	})
}

func strategyMCTSFixture() *mcts.CausalMCTS {
	return mcts.NewCausalMCTS(
		NewCausalEngineAdapter(),
		math.Sqrt2,
		1,
		mctsMinimumCausalRows,
		2,
		3,
		[]int{0, 1},
		[]int{0, 1, 2},
		false,
	)
}

func strategyRowsFixture(treatmentDirection float64) [][]float64 {
	rows := make([][]float64, mctsMinimumCausalRows)

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

func BenchmarkStrategyStateSelectAction(b *testing.B) {
	engine := strategyMCTSFixture()
	rows := strategyRowsFixture(1)
	state := StrategyState{Treatment: 1, CanEnter: true}
	b.ReportAllocs()

	for b.Loop() {
		_, err := state.SelectAction(engine, rows)

		if err != nil {
			b.Fatal(err)
		}
	}
}
