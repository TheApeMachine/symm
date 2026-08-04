package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStrategyStateTrace(t *testing.T) {
	Convey("Given the root state passed to causal MCTS", t, func() {
		state := StrategyState{
			Symbol:        "BTC/USD",
			Energy:        0.2,
			Surprise:      0.01,
			Treatment:     0.004,
			RoundTripCost: 0.0015,
			MaxSteps:      mctsHorizonSteps,
		}

		Convey("It should expose the exact state and a searchable history", func() {
			trace := state.Trace(
				mctsMinimumCausalRows,
				mctsMinimumCausalRows,
				mctsSearchIterations,
			)

			So(trace.Energy, ShouldEqual, state.Energy)
			So(trace.Surprise, ShouldEqual, state.Surprise)
			So(trace.Treatment, ShouldEqual, state.Treatment)
			So(trace.RoundTripCost, ShouldEqual, state.RoundTripCost)
			So(trace.CausalRows, ShouldEqual, mctsMinimumCausalRows)
			So(trace.MinimumCausalRows, ShouldEqual, mctsMinimumCausalRows)
			So(trace.Iterations, ShouldEqual, mctsSearchIterations)
			So(trace.HorizonSteps, ShouldEqual, mctsHorizonSteps)
			So(trace.Searchable, ShouldBeTrue)
		})

		Convey("It should make insufficient causal support explicit", func() {
			trace := state.Trace(
				mctsMinimumCausalRows-1,
				mctsMinimumCausalRows,
				mctsSearchIterations,
			)

			So(trace.Searchable, ShouldBeFalse)
			So(trace.Attempted, ShouldBeFalse)
			So(trace.RecommendedAction, ShouldEqual, types.Action(""))
		})
	})
}

func TestStrategyAction(t *testing.T) {
	Convey("Given each search action", t, func() {
		Convey("It should publish only live planner actions", func() {
			So(strategyAction(ActionNothing), ShouldEqual, types.ActionNothing)
			So(strategyAction(ActionEnter), ShouldEqual, types.ActionEnter)
			So(strategyAction(ActionHold), ShouldEqual, types.ActionHold)
			So(strategyAction(ActionCompleteTrajectory), ShouldEqual, types.Action(""))
		})

		Convey("It should leave an unknown search result absent", func() {
			So(strategyAction(-1), ShouldEqual, types.Action(""))
		})
	})
}

func BenchmarkStrategyStateTrace(b *testing.B) {
	state := StrategyState{
		Symbol:        "BTC/USD",
		Energy:        0.2,
		Surprise:      0.01,
		Treatment:     0.004,
		RoundTripCost: 0.0015,
		MaxSteps:      mctsHorizonSteps,
	}

	b.ResetTimer()

	for range b.N {
		_ = state.Trace(
			mctsMinimumCausalRows,
			mctsMinimumCausalRows,
			mctsSearchIterations,
		)
	}
}

func BenchmarkStrategyAction(b *testing.B) {
	for range b.N {
		_ = strategyAction(ActionCompleteTrajectory)
	}
}
