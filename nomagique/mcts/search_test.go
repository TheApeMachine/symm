package mcts

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
fixedMarketModel is a deterministic market model: constant log return per
step. It exists to make economic rewards exactly computable.
*/
type fixedMarketModel struct {
	logReturn float64
	noise     float64
	steps     int
	value     map[relation.Coordinate]float64
}

func (model *fixedMarketModel) Step(current MarketState) (MarketState, float64, float64, error) {
	next := MarketState{
		At:     current.At.Add(time.Second),
		Values: make(map[relation.Coordinate]float64, len(current.Values)),
	}

	for coordinate, value := range current.Values {
		next.Values[coordinate] = value
	}

	if model.value != nil {
		for coordinate, value := range model.value {
			next.Values[coordinate] = value * math.Exp(model.logReturn)
		}
	}

	return next, model.logReturn, model.noise, nil
}

/*
alwaysEstimable is an action estimator that accepts every action.
*/
type alwaysEstimable struct{}

func (alwaysEstimable) EstimateAction(state State, action Action) ActionEstimate {
	return ActionEstimate{
		Action:             action,
		IdentificationStatus: causal.IdentificationIdentified,
		Defined:            true,
	}
}

/*
neverEstimable marks every action undefined.
*/
type neverEstimable struct{}

func (neverEstimable) EstimateAction(state State, action Action) ActionEstimate {
	return ActionEstimate{
		Action:               action,
		IdentificationStatus: causal.IdentificationNotIdentifiable,
		Defined:              false,
	}
}

func testMarketState(price float64) MarketState {
	return MarketState{
		At:     time.Unix(0, 0),
		Values: map[relation.Coordinate]float64{},
	}
}

func TestEconomicReward(t *testing.T) {
	Convey("Given a deterministic market path and known fees", t, func() {
		marketModel := &fixedMarketModel{logReturn: 0, noise: 0}
		portfolio := PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100}
		state := NewEconomicState(
			portfolio,
			testMarketState(100),
			marketModel,
			CostModel{FeeRate: 0.001, SpreadFraction: 0.0005, SlippageFraction: 0},
			1,
			1,
			1,
		)

		Convey("MCTS reward equals the actual net-wealth change", func() {
			search := NewSearch(4, 0, 1)
			result := search.Run(state, alwaysEstimable{})

			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.SelectedAction, ShouldEqual, Wait)

			// Enter at a constant price with a 0.15% total cost: exact
			// wealth change is -notional * totalCostFraction.
			expectedEnter := -100 * 0.0015
			So(result.ExpectedEconomicOutcome, ShouldEqual, 0)

			for _, alternative := range result.Alternatives {
				if alternative.Action == Enter {
					So(alternative.ExpectedOutcome, ShouldEqual, 0)
					So(alternative.Defined, ShouldBeTrue)
				}
			}

			entered, err := state.ApplyAction(Enter)
			So(err, ShouldBeNil)
			So(entered.GetReward(), ShouldAlmostEqual, expectedEnter, 1e-9)
		})

		Convey("a positive expected return makes Enter the selected action", func() {
			risingModel := &fixedMarketModel{logReturn: 0.01, noise: 0}
			risingState := NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				risingModel,
				CostModel{FeeRate: 0.001, SpreadFraction: 0.0005, SlippageFraction: 0},
				1,
				1,
				3,
			)

			search := NewSearch(64, 0.5, 7)
			result := search.Run(risingState, alwaysEstimable{})
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.SelectedAction, ShouldEqual, Enter)
			So(result.ExpectedEconomicOutcome, ShouldBeGreaterThan, 0)
			So(result.Visits, ShouldBeGreaterThan, 0)
			So(result.Trace, ShouldNotBeNil)
			So(result.Trace.Horizon, ShouldEqual, 3)
		})
	})
}

func TestActionDoesNotMutateMarket(t *testing.T) {
	Convey("Given the same market model", t, func() {
		model := &fixedMarketModel{logReturn: 0.005, noise: 0}

		Convey("Enter and Wait evolve the market identically", func() {
			enterState := NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				model,
				CostModel{FeeRate: 0.001},
				1,
				1,
				2,
			)
			waitState := NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				model,
				CostModel{FeeRate: 0.001},
				1,
				1,
				2,
			)

			entered, err := enterState.ApplyAction(Enter)
			So(err, ShouldBeNil)
			waited, err := waitState.ApplyAction(Wait)
			So(err, ShouldBeNil)

			Convey("portfolio differs because exposure differs", func() {
				So(entered.(*EconomicState).Portfolio.Position, ShouldEqual, 1)
				So(waited.(*EconomicState).Portfolio.Position, ShouldEqual, 0)
			})

			Convey("market state is identical", func() {
				So(entered.(*EconomicState).Market.At, ShouldEqual, waited.(*EconomicState).Market.At)
			})

			Convey("wealth differs because exposure differs", func() {
				So(entered.GetReward(), ShouldNotEqual, waited.GetReward())
			})
		})
	})
}

func TestUndefinedActionEstimate(t *testing.T) {
	Convey("Given no estimable action", t, func() {
		state := NewEconomicState(
			PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			testMarketState(100),
			&fixedMarketModel{logReturn: 0, noise: 0},
			CostModel{},
			1,
			1,
			2,
		)

		search := NewSearch(4, 0.5, 1)
		result := search.Run(state, neverEstimable{})

		Convey("the search returns DecisionUnavailable, not Wait", func() {
			So(result.DecisionUnavailable, ShouldBeTrue)
			So(result.IdentificationStatus, ShouldEqual, causal.IdentificationNotIdentifiable)
			So(result.SelectedAction, ShouldEqual, Action(0))
			So(len(result.UndefinedActions), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSimulationIsNotObservation(t *testing.T) {
	Convey("Given MCTS rollouts over an economic state", t, func() {
		state := NewEconomicState(
			PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			testMarketState(100),
			&fixedMarketModel{logReturn: 0.001, noise: 0.1},
			CostModel{FeeRate: 0.001},
			1,
			1,
			4,
		)

		search := NewSearch(32, 0.5, 3)
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
				&fixedMarketModel{logReturn: 0.002, noise: 0.05},
				CostModel{FeeRate: 0.001, SpreadFraction: 0.0005},
				1,
				2,
				3,
			)

			search := NewSearch(24, 0.5, 99)
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
