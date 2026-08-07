package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func testStrategyForecast(testingContext testing.TB, curve []float64) *types.ResonanceForecast {
	testingContext.Helper()
	retention := make([]float64, len(curve))

	for index := range retention {
		retention[index] = 1
	}

	forecast, err := types.NewResonanceForecast(
		curve, retention, len(curve), 0.75,
	)

	So(err, ShouldBeNil)

	return forecast
}

func TestStrategyStateTrace(t *testing.T) {
	Convey("Given the root state passed to causal MCTS", t, func() {
		forecastHorizon := 7
		forecastCurve := make([]float64, forecastHorizon)

		for index := range forecastCurve {
			forecastCurve[index] = 0.004
		}

		state := StrategyState{
			Symbol:    "BTC/USD",
			Treatment: 0.004,
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
	Convey("Given a multi-step trajectory with live survival and propagation", t, func() {
		forecast, err := types.NewResonanceForecast(
			[]float64{0.004, 0.006, -0.004},
			[]float64{1, 0.75, 0.5},
			3,
			0.75,
		)
		So(err, ShouldBeNil)
		state := StrategyState{
			Treatment: forecast.Curve[0],
		}

		entered := state.ApplyAction(ActionEnter).(StrategyState)
		next := entered.ApplyAction(ActionHold).(StrategyState)

		Convey("Each hold should consume its own supported curve step", func() {
			secondStep, present := forecast.Step(1)
			So(present, ShouldBeTrue)
			propagated := secondStep * state.holdPropagation()
			So(next.Treatment, ShouldAlmostEqual, propagated)
			So(next.Reward, ShouldAlmostEqual,
				forecast.Curve[0]-state.RoundTripCost+propagated)
			So(next.GetInterventionLevel(ActionHold), ShouldAlmostEqual, propagated)

			Convey("A later negative step should not repeat the positive first step", func() {
				second := next.ApplyAction(ActionHold).(StrategyState)
				thirdStep, present := forecast.Step(2)
				So(present, ShouldBeTrue)
				expectedThird := thirdStep * math.Pow(state.holdPropagation(), 2)

				So(second.Treatment, ShouldAlmostEqual, expectedThird)
				So(second.Treatment, ShouldBeLessThan, 0)
				So(second.Reward, ShouldAlmostEqual,
					forecast.Curve[0]-state.RoundTripCost+propagated+expectedThird)
			})
		})
	})

	Convey("Given a forecast without the step requested by a rollout", t, func() {
		forecast := testStrategyForecast(t, []float64{0.004, 0.006})
		forecast.Curve = forecast.Curve[:1]
		state := StrategyState{
			Treatment: 0.004,
			Reward:    0.003,
		}

		next := state.ApplyAction(ActionHold).(StrategyState)

		Convey("It should close the unsupported rollout without fabricating return", func() {
			So(next.IsTerminal(), ShouldBeTrue)
			So(next.Reward, ShouldEqual, state.Reward)
			So(next.Treatment, ShouldEqual, state.Treatment)
		})
	})
}

func BenchmarkStrategyStateTrace(b *testing.B) {
	forecastHorizon := 7
	forecastCurve := make([]float64, forecastHorizon)

	for index := range forecastCurve {
		forecastCurve[index] = 0.004
	}

	state := StrategyState{
		Symbol:    "BTC/USD",
		Treatment: 0.004,
	}

	for b.Loop() {
		_ = state.Trace(
			mctsMinimumCausalRows,
			mctsMinimumCausalRows,
			mctsSearchIterations,
		)
	}
}

func BenchmarkStrategyStateHoldPropagation(b *testing.B) {
	state := StrategyState{}

	for b.Loop() {
		_ = state.holdPropagation()
	}
}

func BenchmarkStrategyAction(b *testing.B) {
	for b.Loop() {
		_ = strategyAction(ActionCompleteTrajectory)
	}
}

func BenchmarkStrategyStateApplyAction(b *testing.B) {
	forecastCurve := make([]float64, 7)

	for index := range forecastCurve {
		forecastCurve[index] = 0.004
	}

	state := StrategyState{
		Treatment: 0.004,
	}

	for b.Loop() {
		_ = state.ApplyAction(ActionHold)
	}
}
