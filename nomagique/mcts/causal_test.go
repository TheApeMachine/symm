package mcts

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
recordingEngine is a causal engine that returns fixed answers and records how
it was queried, so tests can assert the search consults the structural model
with the levels and columns its policy declares.
*/
type recordingEngine struct {
	expectation     float64
	expectationErr  error
	counterfactual  float64
	precision       float64
	counterErr      error
	doLevels        []float64
	abductionLevels []float64
	abductionRows   [][]float64
	historyWidths   []int
}

func (engine *recordingEngine) DoExpectation(
	rows [][]float64,
	target int,
	minimumRows int,
	treatment int,
	level float64,
	controls []int,
) (float64, error) {
	engine.doLevels = append(engine.doLevels, level)
	engine.historyWidths = append(engine.historyWidths, len(rows))

	if engine.expectationErr != nil {
		return 0, engine.expectationErr
	}

	return engine.expectation, nil
}

func (engine *recordingEngine) AbductiveCounterfactual(
	rows [][]float64,
	target int,
	minimumRows int,
	features []int,
	linear bool,
	actual []float64,
	treatment int,
	level float64,
) (float64, float64, float64, error) {
	engine.abductionLevels = append(engine.abductionLevels, level)
	engine.abductionRows = append(engine.abductionRows, append([]float64(nil), actual...))

	if engine.counterErr != nil {
		return 0, 0, 0, engine.counterErr
	}

	return engine.counterfactual, 0, engine.precision, nil
}

func sampleObservationalHistory(count int) [][]float64 {
	rows := make([][]float64, count)

	for index := 0; index < count; index++ {
		rows[index] = []float64{
			100.0 + float64(index)*0.1,
			float64(index),
			1.0,
			0.5 + float64(index)*0.05,
		}
	}

	return rows
}

/*
causalSearchState builds a state with enough horizon for the tree to branch.
*/
func causalSearchState() *EconomicState {
	return NewEconomicState(
		PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
		testMarketState(100),
		priceModel(0.001, 0.05),
		CostModel{FeeRate: 0.001},
		1,
		4,
		4,
	).WithHistory(sampleObservationalHistory(8))
}

func TestCounterfactualUpdatesUntakenSiblings(t *testing.T) {
	Convey("Given a search backed by a structural model", t, func() {
		engine := &recordingEngine{counterfactual: 5, precision: 1}

		search := NewSearch(24, 0.5, 0.25, 11)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 0, 0, true)

		result := search.Run(causalSearchState(), alwaysEstimable{})

		Convey("the search completes on its economic objective", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.Tree, ShouldNotBeNil)
		})

		Convey("untaken branches accrue virtual experience without rollouts", func() {
			counterfactual := 0.0

			for _, child := range result.Tree.Children {
				counterfactual += child.CounterfactualMass
			}

			So(counterfactual, ShouldBeGreaterThan, 0)
			So(len(engine.abductionLevels), ShouldBeGreaterThan, 0)
		})

		Convey("virtual mass never contaminates observed reward statistics", func() {
			for _, child := range result.Tree.Children {
				// The observed mean is the mean of real rollouts only. A
				// counterfactual of +5 would visibly drag it if virtual
				// experience were mixed into the Welford accumulators.
				So(math.IsNaN(child.MeanReward()), ShouldBeFalse)

				if child.Visits == 0 {
					So(child.MeanReward(), ShouldEqual, 0)
				}
			}
		})
	})
}

func TestCounterfactualInformsUnvisitedBranchValue(t *testing.T) {
	Convey("Given a branch with counterfactual mass but no rollouts", t, func() {
		node := &SearchNode{}
		node.CounterfactualReward = 8
		node.CounterfactualMass = 2

		Convey("its blended value falls back to the counterfactual mean", func() {
			So(node.CounterfactualMean(), ShouldEqual, 4)
			So(node.BlendedValue(), ShouldEqual, 4)
		})

		Convey("the observed mean stays untouched at zero", func() {
			So(node.MeanReward(), ShouldEqual, 0)
			So(node.EffectiveVisits(), ShouldEqual, 2)
		})

		Convey("real rollouts blend against virtual mass by weight", func() {
			node.Visits = 2
			node.Mean = 0

			// Two real visits at 0 against two virtual at 4.
			So(node.BlendedValue(), ShouldEqual, 2)
		})
	})
}

func TestCounterfactualMassIsCapped(t *testing.T) {
	Convey("Given a policy that caps virtual experience", t, func() {
		engine := &recordingEngine{counterfactual: 3, precision: 1}

		search := NewSearch(40, 0.5, 0.25, 13)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 0, 1.5, true)

		result := search.Run(causalSearchState(), alwaysEstimable{})
		So(result.Tree, ShouldNotBeNil)

		Convey("no branch exceeds the declared cap", func() {
			for _, child := range result.Tree.Children {
				So(child.CounterfactualMass, ShouldBeLessThanOrEqualTo, 1.5+1e-9)
			}
		})
	})
}

func TestDoExpectationBiasesSelection(t *testing.T) {
	Convey("Given a structural model that favors holding exposure", t, func() {
		engine := &recordingEngine{expectation: 1, precision: 1}

		search := NewSearch(24, 0.5, 0.25, 17)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 2.5, 0, true)

		result := search.Run(causalSearchState(), alwaysEstimable{})
		So(result.DecisionUnavailable, ShouldBeFalse)

		Convey("the interventional query runs at declared treatment levels", func() {
			So(len(engine.doLevels), ShouldBeGreaterThan, 0)

			// Enter from flat intervenes at the unit quantity; Wait from
			// flat intervenes at the current (zero) exposure.
			levels := make(map[float64]bool)

			for _, level := range engine.doLevels {
				levels[level] = true
			}

			So(levels[1] || levels[0], ShouldBeTrue)
		})

		Convey("branches record their interventional expectation", func() {
			recorded := false

			for _, child := range result.Tree.Children {
				if child.CausalExpectationDefined {
					recorded = true
					So(child.CausalExpectation, ShouldEqual, 1)
				}
			}

			So(recorded, ShouldBeTrue)
		})
	})
}

func TestFailedCausalQueryIsNotAFabricatedZero(t *testing.T) {
	Convey("Given a structural model whose queries all fail", t, func() {
		engine := &recordingEngine{
			expectationErr: errors.New("not identifiable"),
			counterErr:     errors.New("not identifiable"),
		}

		search := NewSearch(24, 0.5, 0.25, 19)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 2.5, 0, true)

		result := search.Run(causalSearchState(), alwaysEstimable{})

		Convey("the search still returns an observational decision", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
		})

		Convey("no branch claims a defined causal expectation", func() {
			for _, child := range result.Tree.Children {
				So(child.CausalExpectationDefined, ShouldBeFalse)
			}
		})

		Convey("no branch accrues virtual experience", func() {
			for _, child := range result.Tree.Children {
				So(child.CounterfactualMass, ShouldEqual, 0)
			}
		})
	})
}

func TestSearchWithoutCausalEngineStaysObservational(t *testing.T) {
	Convey("Given a search with no structural model", t, func() {
		search := NewSearch(24, 0.5, 0.25, 23)

		result := search.Run(causalSearchState(), alwaysEstimable{})

		Convey("it decides on rollout evidence alone", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.Tree, ShouldNotBeNil)

			for _, child := range result.Tree.Children {
				So(child.CounterfactualMass, ShouldEqual, 0)
				So(child.CausalExpectationDefined, ShouldBeFalse)
			}
		})
	})
}

func TestInterventionLevelIsNotTheActionOrdinal(t *testing.T) {
	Convey("Given an economic state", t, func() {
		flat := causalSearchState()

		Convey("actions map to exposure levels, not enum ordinals", func() {
			level, defined := flat.GetInterventionLevel(Enter)
			So(defined, ShouldBeTrue)
			So(level, ShouldEqual, flat.UnitQuantity)

			level, defined = flat.GetInterventionLevel(Wait)
			So(defined, ShouldBeTrue)
			So(level, ShouldEqual, 0)
		})

		Convey("infeasible actions declare no level at all", func() {
			_, defined := flat.GetInterventionLevel(Exit)
			So(defined, ShouldBeFalse)

			_, defined = flat.GetInterventionLevel(Scale)
			So(defined, ShouldBeFalse)
		})
	})
}

func TestRolloutTrajectoryDoesNotContaminateObservationalEvidence(t *testing.T) {
	Convey("Given a search that rolls out", t, func() {
		engine := &recordingEngine{expectation: 0.5, precision: 1}

		search := NewSearch(16, 0.5, 0.25, 29)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 1, 0, true)

		search.Run(causalSearchState(), alwaysEstimable{})

		Convey("the evidence table does not accumulate simulated rollouts", func() {
			So(len(engine.historyWidths), ShouldBeGreaterThan, 1)

			first := engine.historyWidths[0]
			last := engine.historyWidths[len(engine.historyWidths)-1]
			So(last, ShouldEqual, first)
		})

		Convey("abduction receives full-width economic rows", func() {
			for _, row := range engine.abductionRows {
				So(len(row), ShouldEqual, EconomicColumnWidth)
			}
		})
	})
}

/*
isFinite reports whether a value is a usable real number.
*/
func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
errAllQueriesFail stands in for a structural model that can identify nothing.
*/
var errAllQueriesFail = errors.New("mcts: not identifiable")
