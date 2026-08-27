package mcts

import (
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

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
	nextAt := current.At.Add(time.Second)
	next := MarketState{
		At:      nextAt,
		Current: make(map[relation.Coordinate]float64, len(current.Current)),
		History: make(map[relation.Coordinate][]MarketSample, len(current.History)),
	}

	for coordinate, value := range current.Current {
		next.Current[coordinate] = value
	}

	for coordinate, samples := range current.History {
		next.History[coordinate] = append([]MarketSample(nil), samples...)
	}

	logReturn := model.logReturn

	if model.noise > 0 && random != nil {
		// Sample the transition noise into the step's return so the model
		// walks a distribution of plausible paths.
		logReturn += model.noise * random.NormFloat64()
	}

	if model.value != nil {
		for coordinate, value := range model.value {
			next.Current[coordinate] = value * math.Exp(logReturn)
			next.History[coordinate] = append(next.History[coordinate], MarketSample{At: nextAt, Value: next.Current[coordinate]})
		}
	}

	return next, logReturn, model.noise, nil
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
		IdentificationStatus: IdentificationIdentified,
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
		IdentificationStatus: IdentificationNotIdentifiable,
		Defined:              false,
	}
}

func testMarketState(price float64) MarketState {
	return MarketState{
		At:      time.Unix(0, 0),
		Current: map[relation.Coordinate]float64{testPriceCoordinate: price},
	}
}

func TestValueAt(t *testing.T) {
	Convey("Given a market state with timestamped history", t, func() {
		at := time.Unix(0, 100*int64(time.Second))

		state := MarketState{
			At: at,
			Current: map[relation.Coordinate]float64{
				testPriceCoordinate: 9,
			},
			History: map[relation.Coordinate][]MarketSample{
				testPriceCoordinate: {
					{At: at.Add(-2 * time.Second), Value: 5},
					{At: at.Add(-1 * time.Second), Value: 6},
					{At: at, Value: 9},
				},
			},
		}

		Convey("a lag matching a stored timestamp reads that exact slice", func() {
			value, found := state.ValueAt(testPriceCoordinate, 1*time.Second)
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 6)
		})

		Convey("a lag falling between stored timestamps clamps to the nearest prior slice", func() {
			// 1.5s is between the 2s and 1s slices; the nearest slice at or
			// before the cutoff is the 2s slice (value 5), never a fabricated
			// zero and never a future value.
			value, found := state.ValueAt(testPriceCoordinate, 1500*time.Millisecond)
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 5)
		})

		Convey("a zero lag reads the current value", func() {
			value, found := state.ValueAt(testPriceCoordinate, 0)
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 9)
		})

		Convey("a lag beyond the retained history reports not-found", func() {
			_, found := state.ValueAt(testPriceCoordinate, 10*time.Second)
			So(found, ShouldBeFalse)
		})
	})
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

			exited, err := entered.ApplyAction(Exit, nil)
			So(err, ShouldBeNil)
			expectedRoundTrip := -2 * (100 * 0.0015)
			So(exited.GetReward(), ShouldAlmostEqual, expectedRoundTrip, 1e-9)
			So(exited.(*EconomicState).Portfolio.Cash, ShouldAlmostEqual, 10000+expectedRoundTrip, 1e-9)
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

				for coordinate, value := range enteredMarket.Current {
					So(waitedMarket.Current[coordinate], ShouldEqual, value)
				}

				Convey("the price coordinate actually evolved", func() {
					So(enteredMarket.Current[testPriceCoordinate], ShouldNotEqual, 100)
				})
			})

			Convey("wealth differs because exposure differs", func() {
				So(entered.GetReward(), ShouldNotEqual, waited.GetReward())
			})
		})
	})
}
