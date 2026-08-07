package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

func TestPlannerSearch(t *testing.T) {
	Convey("Given a causal artifact that has not reached its data requirement", t, func() {
		planner := &Planner{mctsEngine: mcts.NewCausalMCTS(
			NewCausalEngineAdapter(),
			math.Sqrt2,
			1,
			mctsMinimumCausalRows,
			2,
			3,
			[]int{0, 1},
			[]int{0, 1, 2},
			false,
		)}

		decision, err := planner.search("BTC/USD", map[string]any{"ready": false})

		Convey("Then it should write the standing-aside decision", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Symbol, ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given causal analysis that is ready before its MCTS rows are usable", t, func() {
		planner := &Planner{mctsEngine: mcts.NewCausalMCTS(
			NewCausalEngineAdapter(),
			math.Sqrt2,
			1,
			mctsMinimumCausalRows,
			2,
			3,
			[]int{0, 1},
			[]int{0, 1, 2},
			false,
		)}

		decision, err := planner.search("BTC/USD", map[string]any{
			"ready":          true,
			"historyRows":    [][]float64{{1, 2, 3, 4}},
			"treatmentLevel": 3.0,
		})

		Convey("Then it should stand aside until the causal search can run", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func TestPlannerDecisions(t *testing.T) {
	Convey("Given a thesis with causal evidence that is not ready", t, func() {
		planner := &Planner{}
		thesis := types.NewThesis(nil)
		thesis.Causal.Store("BTC/USD", map[string]any{"ready": false})

		decisions := planner.decisions(thesis)

		Convey("Then Planner should write Do Not Enter to the thesis", func() {
			So(decisions, ShouldHaveLength, 1)
			So(decisions[0].Action, ShouldEqual, types.ActionNothing)

			stored, found := thesis.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(stored.(*types.Decision).Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func TestPlannerAdmit(t *testing.T) {
	Convey("Given an Enter verdict from causal search", t, func() {
		planner := &Planner{minimumConfidence: 0.80}
		thesis := types.NewThesis(nil)

		Convey("Then a forecast below the configured confidence becomes Do Not Enter", func() {
			thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
				Forecast: forecastFixture(t, 0.79),
			})

			decision := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(decision.Action, ShouldEqual, types.ActionNothing)
		})

		Convey("Then a forecast at the configured confidence stays attached to Enter", func() {
			forecast := forecastFixture(t, 0.80)
			thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
				Forecast: forecast,
			})

			decision := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Forecast, ShouldEqual, forecast)
			So(decision.Confidence, ShouldEqual, 0.80)
		})
	})
}

func forecastFixture(t *testing.T, confidence float64) *types.ResonanceForecast {
	t.Helper()
	forecast, err := types.NewResonanceForecast(
		[]float64{-0.01, 0.03},
		[]float64{1, 1},
		2,
		confidence,
	)

	if err != nil {
		t.Fatalf("forecast: %v", err)
	}

	return forecast
}

func BenchmarkPlannerDecisions(b *testing.B) {
	planner := &Planner{}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Causal.Store("BTC/USD", map[string]any{"ready": false})
		_ = planner.decisions(thesis)
	}
}
