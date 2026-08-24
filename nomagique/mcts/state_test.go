package mcts

import (
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
testPriceCoordinate is the coordinate the deterministic fixtures evolve.
*/
var testPriceCoordinate = relation.Coordinate{
	Symbol: "TEST/USD",
	Source: "test",
	Metric: "price",
	Epoch:  1,
}

/*
fixedMarketModel is a deterministic market model: constant log return per
step. It exists to make economic rewards exactly computable. Its value map
carries the evolving price coordinate so market-evolution assertions are
meaningful.
*/
type fixedMarketModel struct {
	logReturn float64
	noise     float64
	steps     int
	value     map[relation.Coordinate]float64
}

func (model *fixedMarketModel) Step(current MarketState, random *rand.Rand) (MarketState, float64, float64, error) {
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
priceModel builds a deterministic market model whose value map contains the
price coordinate, so Step evolves a real coordinate and market-evolution
assertions are meaningful.
*/
func priceModel(logReturn float64, noise float64) *fixedMarketModel {
	return &fixedMarketModel{
		logReturn: logReturn,
		noise:     noise,
		value:     map[relation.Coordinate]float64{testPriceCoordinate: 100},
	}
}

/*
alwaysEstimable is an action estimator that accepts every action.
*/
type alwaysEstimable struct{}

func (alwaysEstimable) EstimateAction(state State, action Action) ActionEstimate {
	return ActionEstimate{
		Action:               action,
		IdentificationStatus: causal.IdentificationIdentified,
		Defined:              true,
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
		Values: map[relation.Coordinate]float64{testPriceCoordinate: price},
	}
}

func TestEconomicReward(t *testing.T) {
	Convey("Given a deterministic market path and known fees", t, func() {
		marketModel := priceModel(0, 0)
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
			search := NewSearch(4, 0, 0, 1)
			result := search.Run(state, alwaysEstimable{})

			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.SelectedAction, ShouldEqual, Wait)

			// Enter at a constant price with a 0.15% total cost: exact
			// wealth change is -notional * totalCostFraction.
			expectedEnter := -100 * 0.0015
			So(result.ExpectedEconomicOutcome, ShouldEqual, 0)
			So(expectedEnter, ShouldBeLessThan, 0)

			for _, alternative := range result.Alternatives {
				if alternative.Action == Enter {
					So(alternative.ExpectedOutcome, ShouldEqual, 0)
					So(alternative.Defined, ShouldBeTrue)
				}
			}

			entered, err := state.ApplyAction(Enter, nil)
			So(err, ShouldBeNil)
			So(entered.GetReward(), ShouldAlmostEqual, expectedEnter, 1e-9)
		})

		Convey("a positive expected return makes Enter the selected action", func() {
			risingModel := priceModel(0.01, 0)
			risingState := NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				risingModel,
				CostModel{FeeRate: 0.001, SpreadFraction: 0.0005, SlippageFraction: 0},
				1,
				1,
				3,
			)

			search := NewSearch(64, 0.5, 0.25, 7)
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
		model := priceModel(0.005, 0)

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

			entered, err := enterState.ApplyAction(Enter, nil)
			So(err, ShouldBeNil)
			waited, err := waitState.ApplyAction(Wait, nil)
			So(err, ShouldBeNil)

			Convey("portfolio differs because exposure differs", func() {
				So(entered.(*EconomicState).Portfolio.Position, ShouldEqual, 1)
				So(waited.(*EconomicState).Portfolio.Position, ShouldEqual, 0)
			})

			Convey("market state evolves identically for both actions", func() {
				enteredMarket := entered.(*EconomicState).Market
				waitedMarket := waited.(*EconomicState).Market

				So(enteredMarket.At, ShouldEqual, waitedMarket.At)

				for coordinate, value := range enteredMarket.Values {
					So(waitedMarket.Values[coordinate], ShouldEqual, value)
				}

				Convey("the price coordinate actually evolved", func() {
					So(enteredMarket.Values[testPriceCoordinate], ShouldNotEqual, 100)
				})
			})

			Convey("wealth differs because exposure differs", func() {
				So(entered.GetReward(), ShouldNotEqual, waited.GetReward())
			})
		})
	})
}
