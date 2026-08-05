package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStrategyStateTrace(t *testing.T) {
	Convey("Given the root state passed to causal MCTS", t, func() {
		forecastHorizon := 7
		state := StrategyState{
			Symbol:               "BTC/USD",
			Energy:               0.2,
			Surprise:             0.01,
			Treatment:            0.004,
			RoundTripCost:        0.0015,
			HoldDiscount:         0.8,
			HawkesSpectralRadius: 0.6,
			MaxSteps:             forecastHorizon,
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
			So(trace.HoldDiscount, ShouldEqual, state.HoldDiscount)
			So(trace.HawkesSpectralRadius, ShouldEqual, state.HawkesSpectralRadius)
			So(trace.HoldPropagation, ShouldEqual, state.holdPropagation())
			So(trace.CausalRows, ShouldEqual, mctsMinimumCausalRows)
			So(trace.MinimumCausalRows, ShouldEqual, mctsMinimumCausalRows)
			So(trace.Iterations, ShouldEqual, mctsSearchIterations)
			So(trace.HorizonSteps, ShouldEqual, forecastHorizon)
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

func TestStrategyStateHoldPropagation(t *testing.T) {
	Convey("Given exhaustion survival and a fitted branching matrix", t, func() {
		state := StrategyState{
			HoldDiscount:         0.6,
			HawkesSpectralRadius: 0.8,
		}

		Convey("It should carry the surviving parent and expected offspring", func() {
			So(state.holdPropagation(), ShouldAlmostEqual, 0.6*(1+0.8), 1e-12)
		})

		Convey("It should refuse a non-stationary fit", func() {
			state.HawkesSpectralRadius = 1
			So(state.holdPropagation(), ShouldEqual, 0)
		})
	})
}

func TestStrategyStateApplyAction(t *testing.T) {
	Convey("Given a held trajectory with live survival and propagation", t, func() {
		state := StrategyState{
			Treatment:            0.004,
			HoldDiscount:         0.6,
			HawkesSpectralRadius: 0.8,
			MaxSteps:             7,
			IsHolding:            true,
		}

		next := state.ApplyAction(ActionHold).(StrategyState)

		Convey("Holding should credit the same propagated forecast used by intervention", func() {
			propagated := 0.004 * 0.6 * (1 + 0.8)
			So(next.Treatment, ShouldAlmostEqual, propagated)
			So(next.Reward, ShouldAlmostEqual, propagated)
			So(next.GetInterventionLevel(ActionHold), ShouldAlmostEqual, propagated)

			Convey("A second hold should compound the reflexive transition", func() {
				second := next.ApplyAction(ActionHold).(StrategyState)
				So(second.Treatment, ShouldAlmostEqual,
					propagated*state.holdPropagation())
				So(second.Reward, ShouldAlmostEqual,
					propagated+propagated*state.holdPropagation())
			})
		})
	})
}

func BenchmarkStrategyStateTrace(b *testing.B) {
	forecastHorizon := 7
	state := StrategyState{
		Symbol:               "BTC/USD",
		Energy:               0.2,
		Surprise:             0.01,
		Treatment:            0.004,
		RoundTripCost:        0.0015,
		HoldDiscount:         0.8,
		HawkesSpectralRadius: 0.6,
		MaxSteps:             forecastHorizon,
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

func BenchmarkStrategyStateHoldPropagation(b *testing.B) {
	state := StrategyState{
		HoldDiscount:         0.6,
		HawkesSpectralRadius: 0.8,
	}

	for b.Loop() {
		_ = state.holdPropagation()
	}
}

func BenchmarkStrategyAction(b *testing.B) {
	for range b.N {
		_ = strategyAction(ActionCompleteTrajectory)
	}
}

func BenchmarkStrategyStateApplyAction(b *testing.B) {
	state := StrategyState{
		Treatment:            0.004,
		HoldDiscount:         0.6,
		HawkesSpectralRadius: 0.8,
		MaxSteps:             7,
		IsHolding:            true,
	}

	for b.Loop() {
		_ = state.ApplyAction(ActionHold)
	}
}
