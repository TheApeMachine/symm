package mcts

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
)

func TestUndefinedActionEstimate(t *testing.T) {
	Convey("Given no estimable action", t, func() {
		state := NewEconomicState(
			PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			testMarketState(100),
			priceModel(0, 0),
			CostModel{},
			1,
			1,
			2,
		)

		search := NewSearch(4, 0.5, 0.25, 1)
		result := search.Run(state, neverEstimable{})

		Convey("the search returns DecisionUnavailable, not Wait", func() {
			So(result.DecisionUnavailable, ShouldBeTrue)
			So(result.IdentificationStatus, ShouldEqual, causal.IdentificationNotIdentifiable)

			Convey("every feasible action is recorded as undefined", func() {
				So(len(result.UndefinedActions), ShouldEqual, 2)
				So(containsAction(result.UndefinedActions, Wait), ShouldBeTrue)
				So(containsAction(result.UndefinedActions, Enter), ShouldBeTrue)
			})
		})
	})
}

func TestSimulationIsNotObservation(t *testing.T) {
	Convey("Given MCTS rollouts over an economic state", t, func() {
		state := NewEconomicState(
			PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			testMarketState(100),
			priceModel(0.001, 0.1),
			CostModel{FeeRate: 0.001},
			1,
			1,
			4,
		)

		search := NewSearch(32, 0.5, 0.25, 3)
		result := search.Run(state, alwaysEstimable{})

		Convey("the search result carries economic provenance", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.Trace, ShouldNotBeNil)
			So(len(result.Trace.Branches), ShouldBeGreaterThan, 0)
			So(math.IsNaN(result.ExpectedEconomicOutcome), ShouldBeFalse)
			So(math.IsInf(result.ExpectedEconomicOutcome, 0), ShouldBeFalse)
		})
	})
}

func TestReplayDeterminism(t *testing.T) {
	Convey("Given a fixed seed", t, func() {
		build := func() *SearchResult {
			state := NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				priceModel(0.002, 0.05),
				CostModel{FeeRate: 0.001, SpreadFraction: 0.0005},
				1,
				2,
				3,
			)

			search := NewSearch(24, 0.5, 0.25, 99)
			return search.Run(state, alwaysEstimable{})
		}

		Convey("identical inputs produce identical results", func() {
			first := build()
			second := build()

			So(first.SelectedAction, ShouldEqual, second.SelectedAction)
			So(first.ExpectedEconomicOutcome, ShouldEqual, second.ExpectedEconomicOutcome)
			So(first.Visits, ShouldEqual, second.Visits)
		})
	})
}

func containsAction(actions []Action, action Action) bool {
	for _, candidate := range actions {
		if candidate == action {
			return true
		}
	}

	return false
}
