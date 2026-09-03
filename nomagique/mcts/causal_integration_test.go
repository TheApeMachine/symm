package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestDefaultCausalEngineDrivesSearch runs the search against the real
nomagique/causal Table rather than a stub, so the restored causal layer is
verified against the structural model it actually ships with.
*/
func TestDefaultCausalEngineDrivesSearch(t *testing.T) {
	Convey("Given a search backed by the real causal Table", t, func() {
		search := NewSearch(48, 0.5, 0.25, 31)
		search.Causal = DefaultCausalEngine{Linear: true}
		search.CausalPolicy = EconomicCausalPolicy(8, 1.0, 12, true)

		state := NewEconomicState(
			PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			testMarketState(100),
			priceModel(0.004, 0.02),
			CostModel{FeeRate: 0.001},
			1,
			4,
			5,
		)

		result := search.Run(state, alwaysEstimable{})

		Convey("the search reaches an economic decision", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.Tree, ShouldNotBeNil)
			So(result.Trace, ShouldNotBeNil)
		})

		Convey("the real structural model produces finite causal terms", func() {
			for _, child := range result.Tree.Children {
				So(isFinite(child.CounterfactualMass), ShouldBeTrue)
				So(isFinite(child.CounterfactualReward), ShouldBeTrue)
				So(isFinite(child.BlendedValue()), ShouldBeTrue)
				So(child.CounterfactualMass, ShouldBeGreaterThanOrEqualTo, 0)

				if child.CausalExpectationDefined {
					So(isFinite(child.CausalExpectation), ShouldBeTrue)
				}
			}
		})

		Convey("the economic result stays finite under causal augmentation", func() {
			So(isFinite(result.ExpectedEconomicOutcome), ShouldBeTrue)
			So(isFinite(result.OutcomeUncertainty), ShouldBeTrue)
		})
	})
}

/*
TestCausalSearchIsDeterministic confirms the causal layer preserves the
search's reproducibility: the same seed and policy must yield the same
decision, or a trace cannot be audited after the fact.
*/
func TestCausalSearchIsDeterministic(t *testing.T) {
	Convey("Given two identically seeded causal searches", t, func() {
		run := func() *SearchResult {
			search := NewSearch(32, 0.5, 0.25, 37)
			search.Causal = DefaultCausalEngine{Linear: true}
			search.CausalPolicy = EconomicCausalPolicy(8, 1.0, 12, true)

			return search.Run(NewEconomicState(
				PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				testMarketState(100),
				priceModel(0.003, 0.02),
				CostModel{FeeRate: 0.001},
				1,
				4,
				5,
			), alwaysEstimable{})
		}

		first := run()
		second := run()

		Convey("they select the same action with the same statistics", func() {
			So(first.SelectedAction, ShouldEqual, second.SelectedAction)
			So(first.ExpectedEconomicOutcome, ShouldEqual, second.ExpectedEconomicOutcome)
			So(first.Visits, ShouldEqual, second.Visits)
		})
	})
}
