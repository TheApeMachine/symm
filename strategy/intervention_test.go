package strategy_test

import (
	"math"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
recordingEngine stands in for the causal engine so a search can be asked what it
actually intervened with.

It answers every query with a constant, because the question here is not what
the SCM concludes but which level it was asked about. A constant also keeps the
search deterministic: every child scores the same causal bias, so selection is
decided by UCT alone and the recorded levels are the search's own, not an
artefact of one branch being favoured by a noisy fit.
*/
type recordingEngine struct {
	mu           sync.Mutex
	doLevels     []float64
	counterLevel []float64
}

func (engine *recordingEngine) DoExpectation(
	_ [][]float64, _, _, _ int, level float64, _ []int,
) (float64, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	engine.doLevels = append(engine.doLevels, level)

	return 0.0, nil
}

func (engine *recordingEngine) AbductiveCounterfactual(
	_ [][]float64, _, _ int, _ []int, _ bool, _ []float64, _ int, intervention float64,
) (float64, float64, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	engine.counterLevel = append(engine.counterLevel, intervention)

	return 0.0, 0.0, nil
}

func (engine *recordingEngine) levels() []float64 {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	return append(append([]float64{}, engine.doLevels...), engine.counterLevel...)
}

/*
causalHistory builds a table wide enough for the search to run against, laid out
the way the causal stage lays its rows out: energy, surprise, expected return,
realised return.

The expected-return column carries the magnitudes a per-tick forecast actually
carries, which is the whole point of the assertions below: an action enum is
three orders of magnitude outside this column's support.
*/
func causalHistory(rows int, treatment float64) [][]float64 {
	history := make([][]float64, 0, rows)

	for index := range rows {
		drift := float64(index) * 1e-5

		history = append(history, []float64{
			0.4 + drift,
			0.2 + drift,
			treatment + drift,
			treatment*0.8 + drift,
		})
	}

	return history
}

func interventionForecast(
	t *testing.T,
	treatment float64,
	horizon int,
) *types.ResonanceForecast {
	curve := make([]float64, horizon)
	retention := make([]float64, horizon)

	for index := range horizon {
		curve[index] = treatment
		retention[index] = 1
	}

	forecast, err := types.NewResonanceForecast(
		curve, retention, horizon, 0.75,
	)

	So(err, ShouldBeNil)

	return forecast
}

func TestStrategyStateInterventionLevel(t *testing.T) {
	Convey("Given a candidate whose forecast is a fraction of its price", t, func() {
		forecast := interventionForecast(t, 0.0015, 5)
		state := strategy.StrategyState{
			Symbol:        "SIM1/USD",
			Energy:        0.4,
			Surprise:      0.2,
			Treatment:     0.0015,
			Forecast:      forecast,
			RoundTripCost: 0.0008,
			HoldDiscount:  0.8,
		}

		Convey("It should intervene at the level each action commits the position to", func() {
			// Entering commits to the forecast the candidate was priced on.
			So(state.GetInterventionLevel(strategy.ActionEnter), ShouldEqual, 0.0015)

			// A hold child carries the next supported step after entry, adjusted
			// by the propagation evidence applied to that generation.
			entered := state.ApplyAction(strategy.ActionEnter).(strategy.StrategyState)
			held := entered.ApplyAction(strategy.ActionHold).(strategy.StrategyState)
			So(held.GetInterventionLevel(strategy.ActionHold), ShouldAlmostEqual, 0.0012)

			// Standing aside and completing a rollout both commit to no forecast.
			So(state.GetInterventionLevel(strategy.ActionNothing), ShouldEqual, 0.0)
			So(state.GetInterventionLevel(strategy.ActionCompleteTrajectory), ShouldEqual, 0.0)
		})

		Convey("It should never hand the SCM an action enum as a treatment level", func() {
			for _, action := range []float64{
				strategy.ActionNothing,
				strategy.ActionEnter,
				strategy.ActionHold,
				strategy.ActionCompleteTrajectory,
			} {
				level := state.GetInterventionLevel(action)

				So(level, ShouldBeLessThanOrEqualTo, state.Treatment)
				So(math.Abs(level), ShouldBeLessThan, 1.0)
			}
		})
	})
}

/*
TestSearchInterventionLevels drives the production search against a recording
engine, so what is asserted is the level the engine was actually handed rather
than the level the state would have offered if anyone had asked it.

The two are only the same once the search consults the state, which is the wiring
this proves. Before it, every one of these calls carried 1, 2, or 3 — the action
enums — into a column whose observed values are thousandths.
*/
func TestSearchInterventionLevels(t *testing.T) {
	Convey("Given a causal search over a candidate's trajectory", t, func() {
		engine := &recordingEngine{}
		treatment := 0.0015

		search := mcts.NewCausalMCTS(
			engine,
			1.414, 0.5,
			12, 2, 3,
			[]int{0, 1}, []int{0, 1, 2},
			false,
		)

		root := strategy.StrategyState{
			Symbol:        "SIM1/USD",
			Energy:        0.4,
			Surprise:      0.2,
			Treatment:     treatment,
			Forecast:      interventionForecast(t, treatment, 5),
			RoundTripCost: 0.0004,
			HoldDiscount:  0.8,
		}

		action, err := search.Search(root, 50, causalHistory(16, treatment))

		Convey("It should complete and recommend one of the trajectory's actions", func() {
			So(err, ShouldBeNil)
			So([]float64{
				strategy.ActionNothing,
				strategy.ActionEnter,
			}, ShouldContain, action)
		})

		Convey("It should have queried the engine while searching", func() {
			// A search that never reached the engine would pass every assertion
			// below vacuously.
			So(len(engine.levels()), ShouldBeGreaterThan, 0)
		})

		Convey("It should only ever intervene at levels the treatment column carries", func() {
			permitted := map[float64]bool{
				0.0:       true,
				treatment: true,
			}
			trajectory := root.ApplyAction(strategy.ActionEnter).(strategy.StrategyState)
			permitted[trajectory.GetInterventionLevel(strategy.ActionEnter)] = true

			for !trajectory.IsTerminal() {
				trajectory = trajectory.ApplyAction(
					strategy.ActionHold,
				).(strategy.StrategyState)
				permitted[trajectory.GetInterventionLevel(strategy.ActionHold)] = true
			}

			for _, level := range engine.levels() {
				So(permitted[level], ShouldBeTrue)
			}
		})

		Convey("It should never intervene at an action enum", func() {
			for _, level := range engine.levels() {
				So(level, ShouldNotEqual, strategy.ActionEnter)
				So(level, ShouldNotEqual, strategy.ActionHold)
				So(level, ShouldNotEqual, strategy.ActionCompleteTrajectory)

				// Every level stays inside the order of magnitude the column
				// was fitted on. An enum is three orders outside it.
				So(math.Abs(level), ShouldBeLessThanOrEqualTo, treatment)
			}
		})
	})
}
