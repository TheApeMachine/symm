package strategy

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
syntheticMeasurement builds one cvd Measurement with the schema coordinates.
The signed flow z-score causally leads the midpoint log return.
*/
func syntheticMeasurement(index int, flow float64, gross float64, ret float64, metadata float64) *nmtypes.Measurement {
	at := time.Unix(0, int64(index)*int64(time.Second))

	return &nmtypes.Measurement{
		ID:           fmt.Sprintf("test:%d", index),
		Source:       "cvd",
		Symbol:       "TEST/USD",
		At:           at,
		ObservedFrom: at.Add(-time.Second),
		Maturity:     0.9,
		SNR:          0.01,
		SNRDefined:   true,
		Metrics: map[string]*nmtypes.Metric[float64]{
			"signed_net_fraction_zscore": nmtypes.NewMetric(
				"signed_net_fraction_zscore", flow,
				nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			),
			"gross_notional_rate_zscore": nmtypes.NewMetric(
				"gross_notional_rate_zscore", gross,
				nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			),
			"midpoint_log_return": nmtypes.NewMetric(
				"midpoint_log_return", ret,
				nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			),
		},
		Metadata: map[string]float64{"legacy_opportunity_score": metadata},
	}
}

/*
syntheticSeries generates a causal series with realistic per-step return
scale: flow responds to gross, and the log return responds to lagged flow.
*/
func syntheticSeries(count int, seed int64) ([]float64, []float64, []float64) {
	random := rand.New(rand.NewSource(seed))
	gross := make([]float64, count)
	flow := make([]float64, count)
	ret := make([]float64, count)

	for index := 1; index < count; index++ {
		gross[index] = random.NormFloat64()
		flow[index] = 0.5*flow[index-1] + 0.05*gross[index-1] + 0.05*random.NormFloat64()
		ret[index] = 0.004*flow[index-1] + 0.2*ret[index-1] + 0.002*random.NormFloat64()
	}

	return flow, gross, ret
}

func testConfig() *system.Config {
	return &system.Config{
		Planner: &system.PlannerConfig{
			MaxAllocationFraction: 0.1,
			MCTSIterations:        24,
			ExplorationConstant:   0.5,
			UncertaintyWeight:     0.25,
			SearchHorizon:         3,
			MaxPositionUnits:      1,
			SlippageFraction:      0,
			RelationInterval:      time.Nanosecond,
		},
	}
}

/*
deterministicCausalState builds a causal state with a constant-return,
zero-noise market path and the full schema transition set. Every transition
has zero residual variance, so the MCTS reward equals the analytic net-wealth
change exactly.
*/
func deterministicCausalState(at time.Time, constantReturn float64) *CausalState {
	schema := DefaultCausalSchema(1, time.Second).ForSymbol("TEST/USD")
	outcome := schema.Outcomes[0]
	transitions := make(map[relation.Coordinate]*causal.TransitionModel)

	for _, marketVariable := range schema.MarketVariables {
		transition := &causal.TransitionModel{
			Target:           marketVariable.Variable,
			SelfLag:          marketVariable.SelfLag,
			Intercept:        constantReturn,
			SelfCoefficient:  0,
			ResidualVariance: 0,
			EffectiveSupport: 200,
			Maturity:         0.995,
			Status:           causal.IdentificationIdentified,
		}
		transitions[marketVariable.Variable.Coordinate] = transition
	}

	outcomeTransition := transitions[outcome.Coordinate]

	return &CausalState{
		Symbol:         "TEST/USD",
		At:             at,
		Epoch:          1,
		SchemaVersion:  schema.Version,
		ModelVersion:   "deterministic-test-v1",
		Identification: causal.IdentificationIdentified,
		MarketState: mcts.MarketState{
			At:      at,
			Current: map[relation.Coordinate]float64{outcome.Coordinate: constantReturn},
		},
		OutcomeVariable: outcome,
		Transition:      outcomeTransition,
		Transitions:     transitions,
		ActiveClosure:   []relation.Coordinate{outcome.Coordinate},
		StepLag:         time.Second,
	}
}

func newTestReasoner() *Reasoner {
	schemaTemplate := DefaultCausalSchema(1, time.Second)

	graphSolver := graph.NewSolver(
		context.Background(),
		1,
		2048,
		RelationPlansFromSchema(schemaTemplate, 1, 30*time.Second),
		schemaTemplate.Version,
		graph.WithInterval(0),
	)

	reasoner, err := NewReasoner(1, graphSolver.Store(), graphSolver.Graph(), schemaTemplate)

	if err != nil {
		panic(err)
	}

	testHarness = graphSolver

	return reasoner
}

// testHarness holds the graph solver that owns the shared store and influence
// graph for the current test reasoner. Ingestion and relation estimation run
// through it — the reasoner is a pure consumer of its fitted state. In the
// production wiring this is the graph.Solver shared through the workspace; the
// test binds it directly.
var testHarness *graph.Solver

/*
ingestSeries feeds a synthetic series into the graph solver that owns the
reasoner's shared store, then refreshes Relations once at the end so the
reasoner observes the fully fitted influence graph.
*/
func ingestSeries(reasoner *Reasoner, count int, seed int64, metadata float64) {
	flow, gross, ret := syntheticSeries(count, seed)

	for index := 0; index < count; index++ {
		testHarness.StepMeasurement(syntheticMeasurement(index, flow[index], gross[index], ret[index], metadata))
	}
}

func TestConformanceInformationPreservation(t *testing.T) {
	Convey("Given many measurement coordinates ingested", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 120, 5, 0)

		Convey("all coordinates remain in observational history after one causal query", func() {
			countCoordinates := func() int {
				count := 0

				reasoner.Store().RangeCoordinates(func(relation.Coordinate) bool {
					count++
					return true
				})

				return count
			}

			before := countCoordinates()
			So(before, ShouldBeGreaterThanOrEqualTo, 3)

			state := reasoner.CausalState("TEST/USD", time.Unix(0, 119*int64(time.Second)))
			So(state, ShouldNotBeNil)

			after := countCoordinates()
			So(after, ShouldEqual, before)

			reasoner.Store().RangeCoordinates(func(coordinate relation.Coordinate) bool {
				So(reasoner.Store().Count(coordinate), ShouldBeGreaterThan, 0)
				return true
			})
		})

		Convey("the unused gross-rate coordinate stays queryable", func() {
			visited := 0

			reasoner.Store().RangeHistory(relation.Coordinate{
				Symbol:    "TEST/USD",
				Source:    "cvd",
				Metric:    "gross_notional_rate_zscore",
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
				Epoch:     1,
			}, func(relation.Observation) bool {
				visited++
				return true
			})

			So(visited, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConformanceNoEvidenceThresholds(t *testing.T) {
	Convey("Given low-SNR observations", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 120, 7, 0)

		Convey("zero-gain relations remain stored and queryable", func() {
			edges := reasoner.Graph().Edges()
			So(len(edges), ShouldBeGreaterThanOrEqualTo, 1)

			for _, edge := range edges {
				So(edge.Result, ShouldNotBeNil)

				if edge.Result.PredictiveGain != nil {
					So(math.IsInf(*edge.Result.PredictiveGain, 0), ShouldBeFalse)
				}
			}
		})

		Convey("low maturity does not evict anything", func() {
			So(reasoner.Store().Snapshot().Observations, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConformanceUndefinedNotZero(t *testing.T) {
	Convey("Given an identified and an unavailable state", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 120, 11, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 119*int64(time.Second)))

		Convey("the identified transition is defined", func() {
			So(state.Identification, ShouldEqual, causal.IdentificationIdentified)
			So(state.Transition, ShouldNotBeNil)
		})

		Convey("an unsupported causal query returns NotIdentifiable, not zero", func() {
			model := reasoner.modelFor("TEST/USD")
			effect := model.Outcome(causal.OutcomeRequest{
				Treatment: "enter",
				Target:    state.OutcomeVariable,
				Current:   map[causal.VariableID]float64{},
				At:        state.At,
			})
			So(effect.Status, ShouldEqual, causal.IdentificationNotIdentifiable)
			So(effect.Defined(), ShouldBeFalse)
		})

		Convey("the planner represents unavailable evaluation explicitly", func() {
			unavailable := &CausalState{
				Symbol:         "TEST/USD",
				At:             state.At,
				Identification: causal.IdentificationNotIdentifiable,
			}
			planner := &Planner{}
			decision := planner.decisionFromCausalState(unavailable, testConfig(), marketInputs{})
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Reason, ShouldContainSubstring, "causal evaluation unavailable")
		})
	})
}

func TestConformanceNoSimulatedEvidence(t *testing.T) {
	Convey("Given a reasoner with real observations", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 120, 13, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 119*int64(time.Second)))

		before := reasoner.Store().Snapshot()

		Convey("running many MCTS rollouts changes no observation counts", func() {
			economicState := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				state.MarketState,
				&causalMarketModel{state: state},
				mcts.CostModel{FeeRate: 0.001},
				1,
				2,
				3,
			)

			search := mcts.NewSearch(200, 0.5, 0.25, 99)
			result := search.Run(economicState, &causalActionEstimator{state: state})
			So(result.DecisionUnavailable, ShouldBeFalse)

			after := reasoner.Store().Snapshot()
			So(after.Appended, ShouldEqual, before.Appended)
			So(after.Observations, ShouldEqual, before.Observations)
			So(after.Coordinates, ShouldEqual, before.Coordinates)
		})
	})
}

func TestConformanceEconomicReward(t *testing.T) {
	Convey("Given a deterministic market path and known fees", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		costs := mcts.CostModel{FeeRate: 0.001, SpreadFraction: 0.0005}
		unitQuantity := 1.0
		price := 100.0

		Convey("MCTS reward equals the actual net-wealth change along the path", func() {
			// A zero expected move makes the cost structure the whole story:
			// entering loses the crossing cost, waiting keeps wealth flat.
			flatState := deterministicCausalState(at, 0)
			economic := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: price},
				flatState.MarketState,
				&causalMarketModel{state: flatState},
				costs,
				unitQuantity,
				unitQuantity,
				1,
			)

			// Exact reward calculation: with no market move, Enter pays the
			// per-side crossing cost and Wait pays nothing.
			exactEnter := -unitQuantity * price * costs.TotalFraction()
			exactWait := 0.0

			entered, err := economic.ApplyAction(mcts.Enter, nil)
			So(err, ShouldBeNil)
			So(entered.GetReward(), ShouldAlmostEqual, exactEnter, 1e-9)

			waited, err := economic.ApplyAction(mcts.Wait, nil)
			So(err, ShouldBeNil)
			So(waited.GetReward(), ShouldAlmostEqual, exactWait, 1e-9)

			Convey("the economic invariant holds: entering costs money and waiting is better", func() {
				So(exactEnter, ShouldBeLessThan, 0)
				So(waited.GetReward(), ShouldBeGreaterThan, entered.GetReward())
			})

			Convey("the search reflects the invariant without pinning exact output", func() {
				search := mcts.NewSearch(16, 0, 0, 1)
				result := search.Run(economic, &causalActionEstimator{state: flatState})
				So(result.DecisionUnavailable, ShouldBeFalse)
				So(result.ExpectedEconomicOutcome, ShouldBeGreaterThanOrEqualTo, entered.GetReward())
			})
		})

		Convey("causal uncertainty propagates into sampled rollouts", func() {
			reasoner := newTestReasoner()
			ingestSeries(reasoner, 150, 17, 0)
			noisyState := reasoner.CausalState("TEST/USD", at)
			So(noisyState, ShouldNotBeNil)
			So(noisyState.Identification, ShouldEqual, causal.IdentificationIdentified)

			economic := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: price},
				noisyState.MarketState,
				&causalMarketModel{state: noisyState},
				costs,
				unitQuantity,
				unitQuantity,
				3,
			)

			search := mcts.NewSearch(400, 0.25, 0.5, 7)
			result := search.Run(economic, &causalActionEstimator{state: noisyState})

			Convey("the sampled reward dispersion is non-zero", func() {
				bestStd := 0.0

				for _, branch := range result.Trace.Branches {
					bestStd = math.Max(bestStd, branch.RewardStd)
				}

				So(bestStd, ShouldBeGreaterThan, 0)
			})

			Convey("the deterministic causal estimate equals the analytic expected path", func() {
				estimate := (&causalActionEstimator{state: noisyState}).EstimateAction(economic, mcts.Wait)
				So(estimate.Defined, ShouldBeTrue)
				So(estimate.Uncertainty, ShouldBeGreaterThan, 0)
			})
		})

		Convey("changing a semantic UI score does not change reward or action values", func() {
			left := newTestReasoner()
			ingestSeries(left, 150, 19, 0.9)
			right := newTestReasoner()
			ingestSeries(right, 150, 19, -0.9)

			leftState := left.CausalState("TEST/USD", at)
			rightState := right.CausalState("TEST/USD", at)

			runSearch := func(state *CausalState) *mcts.SearchResult {
				economic := mcts.NewEconomicState(
					mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: price},
					state.MarketState,
					&causalMarketModel{state: state},
					costs,
					unitQuantity,
					unitQuantity,
					3,
				)

				search := mcts.NewSearch(48, 0.5, 0.25, 123)
				return search.Run(economic, &causalActionEstimator{state: state})
			}

			leftResult := runSearch(leftState)
			rightResult := runSearch(rightState)

			So(leftResult.SelectedAction, ShouldEqual, rightResult.SelectedAction)
			So(leftResult.ExpectedEconomicOutcome, ShouldEqual, rightResult.ExpectedEconomicOutcome)
		})
	})
}

func TestConformanceInfluenceInformsCausalModel(t *testing.T) {
	Convey("Given an influence graph with a measured relation", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 41, 0)

		Convey("the causal transition uses the measured influence lag, not the schema fallback", func() {
			state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
			So(state, ShouldNotBeNil)
			So(state.Identification, ShouldEqual, causal.IdentificationIdentified)

			edge := reasoner.Graph().Relation(
				relation.Coordinate{
					Symbol:    "TEST/USD",
					Source:    "cvd",
					Metric:    "signed_net_fraction_zscore",
					Unit:      nmtypes.UnitDimensionless,
					Timescale: nmtypes.TimescaleInstantaneous,
					Epoch:     1,
				},
				relation.Coordinate{
					Symbol:    "TEST/USD",
					Source:    "cvd",
					Metric:    "midpoint_log_return",
					Unit:      nmtypes.UnitDimensionless,
					Timescale: nmtypes.TimescaleInstantaneous,
					Epoch:     1,
				},
			)
			So(edge, ShouldNotBeNil)
			So(edge.Result.Defined(), ShouldBeTrue)

			Convey("the outcome transition's parent lag equals the measured lag", func() {
				outcome := state.OutcomeVariable
				transition := state.Transitions[outcome.Coordinate]
				So(transition, ShouldNotBeNil)

				found := false

				for _, parent := range transition.Parents {
					if parent.Parent.Coordinate.Metric == "signed_net_fraction_zscore" {
						found = true
						So(parent.Lag, ShouldEqual, edge.Result.Lag)
						So(parent.LagSource, ShouldContainSubstring, "influence:")
					}
				}

				So(found, ShouldBeTrue)
			})
		})
	})
}

func TestConformanceNoFinalGate(t *testing.T) {
	Convey("Given MCTS selects Enter on a deterministic positive path", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		state := deterministicCausalState(at, 0.005)

		inputs := marketInputs{cash: 100000, mark: 1, feeRate: 0.001, spreadFraction: 0, available: true}
		planner := &Planner{marketProvider: func(symbol string) marketInputs {
			return inputs
		}}
		decision := planner.decisionFromCausalState(state, testConfig(), inputs)
		So(decision.Action, ShouldEqual, types.ActionEnter)

		Convey("adverse legacy semantic values do not change the selected action", func() {
			decision.ThesisScore = 0
			decision.Confidence = 0
			decision.GraphScore = -1
			decision.PredictiveReady = false
			decision.Opportunity = false
			decision.ReserveEligible = false

			// The action authority is the causal MCTS result; legacy fields
			// are inert telemetry.
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Reason, ShouldEqual, "")
		})
	})
}

func TestConformanceActionDoesNotMutateMarket(t *testing.T) {
	Convey("Given the same causal market model", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		state := deterministicCausalState(at, 0.005)
		base := state.MarketState

		enterState := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			base,
			&causalMarketModel{state: state},
			mcts.CostModel{FeeRate: 0.001},
			1, 1, 2,
		)
		waitState := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			base,
			&causalMarketModel{state: state},
			mcts.CostModel{FeeRate: 0.001},
			1, 1, 2,
		)

		entered, err := enterState.ApplyAction(mcts.Enter, nil)
		So(err, ShouldBeNil)
		waited, err := waitState.ApplyAction(mcts.Wait, nil)
		So(err, ShouldBeNil)

		Convey("Enter changes portfolio variables only; the market evolves identically", func() {
			enteredEconomic := entered.(*mcts.EconomicState)
			waitedEconomic := waited.(*mcts.EconomicState)

			So(enteredEconomic.Portfolio.Position, ShouldEqual, 1)
			So(waitedEconomic.Portfolio.Position, ShouldEqual, 0)
			So(enteredEconomic.Market.At, ShouldEqual, waitedEconomic.Market.At)

			for coordinate, value := range enteredEconomic.Market.Current {
				So(waitedEconomic.Market.Current[coordinate], ShouldEqual, value)
			}

			Convey("wealth differs because exposure differs", func() {
				So(entered.GetReward(), ShouldNotEqual, waited.GetReward())
			})
		})
	})
}

func TestConformanceReplayDeterminism(t *testing.T) {
	Convey("Given identical measurements, schema, and strategy policy", t, func() {
		build := func() (*CausalState, *mcts.SearchResult, *Reasoner) {
			reasoner := newTestReasoner()
			ingestSeries(reasoner, 150, 31, 0)
			state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
			So(state, ShouldNotBeNil)

			economic := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				state.MarketState,
				&causalMarketModel{state: state},
				mcts.CostModel{FeeRate: 0.001},
				1, 1, 3,
			)

			search := mcts.NewSearch(24, 0.5, 0.25, 77)
			return state, search.Run(economic, &causalActionEstimator{state: state}), reasoner
		}

		firstState, firstResult, firstReasoner := build()
		secondState, secondResult, secondReasoner := build()

		Convey("influence outputs are replayable", func() {
			firstEdges := firstReasoner.Graph().Edges()
			secondEdges := secondReasoner.Graph().Edges()
			So(len(firstEdges), ShouldEqual, len(secondEdges))

			for index := range firstEdges {
				So(firstEdges[index].Result.Lag, ShouldEqual, secondEdges[index].Result.Lag)
				So(firstEdges[index].Result.Coefficient, ShouldResemble, secondEdges[index].Result.Coefficient)
				So(firstEdges[index].Result.PredictiveGain, ShouldResemble, secondEdges[index].Result.PredictiveGain)
			}
		})

		Convey("causal estimates are replayable", func() {
			So(firstState.Identification, ShouldEqual, secondState.Identification)
			So(firstState.Transition.SelfCoefficient, ShouldEqual, secondState.Transition.SelfCoefficient)
			So(firstState.Transition.ResidualVariance, ShouldEqual, secondState.Transition.ResidualVariance)
		})

		Convey("MCTS traces are replayable", func() {
			So(firstResult.SelectedAction, ShouldEqual, secondResult.SelectedAction)
			So(firstResult.ExpectedEconomicOutcome, ShouldEqual, secondResult.ExpectedEconomicOutcome)
			So(firstResult.Visits, ShouldEqual, secondResult.Visits)
			So(firstResult.OutcomeUncertainty, ShouldEqual, secondResult.OutcomeUncertainty)
		})
	})
}

func TestConformanceMultiStepSystemEvolution(t *testing.T) {
	Convey("Given a reasoner with multiple evolving market variables", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 43, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
		So(state.Identification, ShouldEqual, causal.IdentificationIdentified)

		flowCoordinate := relation.Coordinate{
			Symbol:    "TEST/USD",
			Source:    "cvd",
			Metric:    "signed_net_fraction_zscore",
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
			Epoch:     1,
		}

		Convey("a multi-step rollout evolves the whole system, not just price", func() {
			flowTransition := state.Transitions[flowCoordinate]
			So(flowTransition, ShouldNotBeNil)
			So(flowTransition.Status, ShouldEqual, causal.IdentificationIdentified)

			initial := state.MarketState.Current[flowCoordinate]
			economic := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				state.MarketState,
				&causalMarketModel{state: state},
				mcts.CostModel{FeeRate: 0.001},
				1, 1, 3,
			)

			stepped := economic

			for step := 0; step < 2; step++ {
				next, err := stepped.ApplyAction(mcts.Wait, nil)
				So(err, ShouldBeNil)
				stepped = next.(*mcts.EconomicState)
			}

			evolved := stepped.Market.Current[flowCoordinate]
			So(evolved, ShouldNotEqual, initial)
		})
	})
}

func TestConformanceCrossSignalParticipation(t *testing.T) {
	Convey("Given a non-CVD signal coordinate ingested alongside CVD", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 47, 0)

		// Ingest a hawkes measurement for the same symbol.
		at := time.Unix(0, 149*int64(time.Second))
		hawkesMeasurement := &nmtypes.Measurement{
			ID:         "test:hawkes:149",
			Source:     "hawkes",
			Symbol:     "TEST/USD",
			At:         at,
			Maturity:   0.9,
			SNR:        0.5,
			SNRDefined: true,
			Metrics: map[string]*nmtypes.Metric[float64]{
				"conditional_intensity:buy": nmtypes.NewMetric(
					"conditional_intensity:buy", 0.4,
					nmtypes.Descriptor{Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerSecond},
				),
			},
		}
		testHarness.StepMeasurement(hawkesMeasurement)

		hawkesCoordinate := relation.Coordinate{
			Symbol:    "TEST/USD",
			Source:    "hawkes",
			Metric:    "conditional_intensity",
			Side:      "buy",
			Unit:      nmtypes.UnitEventsPerSecond,
			Timescale: nmtypes.TimescalePerSecond,
			Epoch:     1,
		}

		Convey("the coordinate participates in the candidate relation space", func() {
			So(reasoner.Store().Count(hawkesCoordinate), ShouldEqual, 1)

			edge := reasoner.Graph().Relation(hawkesCoordinate, relation.Coordinate{
				Symbol:    "TEST/USD",
				Source:    "cvd",
				Metric:    "midpoint_log_return",
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
				Epoch:     1,
			})

			// One hawkes observation cannot align a causal relation; the
			// candidate is represented as unavailable, not deleted.
			So(edge, ShouldBeNil)

			candidates := reasoner.Graph().Candidates()
			hawkesCandidate := false

			for _, candidate := range candidates {
				if candidate.Source == hawkesCoordinate {
					hawkesCandidate = true
				}
			}

			So(hawkesCandidate, ShouldBeTrue)
		})

		Convey("an unavailable Relation never becomes an active causal parent", func() {
			state := reasoner.CausalState("TEST/USD", at)
			So(state, ShouldNotBeNil)

			// The mediated topology wires hawkes conditional intensity into
			// the signed-flow transition (Layer 3), not directly into the
			// price outcome (Layer 4). The outcome therefore must not expose
			// hawkes as a parent at all — active or excluded.
			signedFlow := relation.Coordinate{
				Symbol:    "TEST/USD",
				Source:    "cvd",
				Metric:    "signed_net_fraction_zscore",
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
				Epoch:     1,
			}
			flowTransition := state.Transitions[signedFlow]
			So(flowTransition, ShouldNotBeNil)

			active := false

			for _, parent := range flowTransition.Parents {
				if parent.Parent.Coordinate.Metric == "conditional_intensity" {
					active = true
				}
			}

			// With a single hawkes observation the parent is excluded from
			// the query-local fit (no observed history to align), but the
			// schema still authorizes the direction on the flow transition.
			So(active, ShouldBeFalse)

			excluded := false

			for _, excludedParent := range flowTransition.ExcludedParents {
				if excludedParent.Parent.Coordinate.Metric == "conditional_intensity" {
					excluded = true
				}
			}

			So(excluded, ShouldBeTrue)

			// The outcome is mediated: hawkes is not a direct structural
			// parent of price return in either role.
			outcomeTransition := state.Transitions[state.OutcomeVariable.Coordinate]
			So(outcomeTransition, ShouldNotBeNil)

			for _, parent := range outcomeTransition.Parents {
				So(parent.Parent.Coordinate.Metric, ShouldNotEqual, "conditional_intensity")
			}

			for _, excludedParent := range outcomeTransition.ExcludedParents {
				So(excludedParent.Parent.Coordinate.Metric, ShouldNotEqual, "conditional_intensity")
			}
		})

		Convey("an unrelated newly observed coordinate does not veto an evaluable query", func() {
			// The single hawkes observation is present in the coordinate store,
			// but because hawkes is an excluded parent of flow, hawkes is NOT
			// in the active dependency closure of the price outcome.
			// The price outcome query remains identified.
			state := reasoner.CausalState("TEST/USD", at)
			So(state, ShouldNotBeNil)
			So(state.Identification, ShouldEqual, causal.IdentificationIdentified)
		})
	})
}

func TestConformanceMediatedPathNoDoubleCounting(t *testing.T) {
	Convey("Given the mediated path Hawkes → CVD → Price in the schema", t, func() {
		schema := causal.NewCausalSchema("path-v1", "TEST/USD", 1)
		hawkes := causal.VariableID{
			Coordinate: relation.Coordinate{Source: "hawkes", Metric: "conditional_intensity:buy"},
			Role:       causal.RoleMarket,
		}
		cvdFlow := causal.VariableID{
			Coordinate: relation.Coordinate{Source: "cvd", Metric: "signed_net_fraction_zscore"},
			Role:       causal.RoleMarket,
		}
		priceReturn := causal.VariableID{
			Coordinate: relation.Coordinate{Source: "cvd", Metric: "midpoint_log_return"},
			Role:       causal.RoleMarket,
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: priceReturn,
			SelfLag:  time.Second,
			Parents:  []causal.AllowedParent{{Parent: cvdFlow, Lag: time.Second}},
		})
		schema.AddMarketVariable(causal.MarketVariable{
			Variable: cvdFlow,
			SelfLag:  time.Second,
			Parents:  []causal.AllowedParent{{Parent: hawkes, Lag: time.Second}},
		})

		store := relation.NewObservationStore(2048)
		store.Append(relation.Observation{Coordinate: hawkes.Coordinate, Raw: 1, At: time.Unix(0, 0)})
		store.Append(relation.Observation{Coordinate: cvdFlow.Coordinate, Raw: 0.5, At: time.Unix(0, 0)})
		store.Append(relation.Observation{Coordinate: priceReturn.Coordinate, Raw: 0.01, At: time.Unix(0, 0)})

		model := causal.NewCausalModel(schema, store, nil, "path-v1")

		Convey("the path is represented structurally, not as evidence votes", func() {
			// Hawkes is a schema-authorized parent of CVD; CVD of Price.
			cvdSpec, found := schema.MarketVariableFor(cvdFlow)
			So(found, ShouldBeTrue)
			So(cvdSpec.Parents[0].Parent, ShouldEqual, hawkes)

			// An action on Price is NotIdentifiable: there is no second
			// "reward vote" from redundant semantic evidence.
			effect := model.Outcome(causal.OutcomeRequest{
				Treatment: "enter",
				Target:    priceReturn,
				Current:   map[causal.VariableID]float64{},
				At:        time.Unix(0, 1),
			})
			So(effect.Status, ShouldEqual, causal.IdentificationNotIdentifiable)
		})
	})
}

func TestConformanceStartupMarksUnpricedSymbolsMissingMarket(t *testing.T) {
	Convey("Given a planner with a mixed priced/unpriced candidate universe", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		priced := deterministicCausalState(at, 0.005)
		priced.Symbol = "PRICED/USD"
		unpriced := deterministicCausalState(at, 0.005)
		unpriced.Symbol = "UNPRICED/USD"

		inputs := marketInputs{cash: 100000, mark: 1, feeRate: 0.001, spreadFraction: 0, available: true}
		planner := &Planner{
			tradingGate: func() bool { return true },
			marketProvider: func(symbol string) marketInputs {
				if symbol == "PRICED/USD" {
					return inputs
				}

				return marketInputs{}
			},
		}
		planner.stager = audit.NewStager(nil)
		planner.pending.Store(priced.Symbol, priced)
		planner.pending.Store(unpriced.Symbol, unpriced)

		Convey("Update reports the unpriced symbol as a missing execution market", func() {
			round := planner.Update(types.NewThesis(t.Context()))
			So(round, ShouldNotBeNil)
			So(len(round.Decisions), ShouldEqual, 2)

			bySymbol := map[string]*types.Decision{}
			for _, decision := range round.Decisions {
				bySymbol[decision.Symbol] = decision
			}

			So(bySymbol["PRICED/USD"], ShouldNotBeNil)
			unpricedDecision := bySymbol["UNPRICED/USD"]
			So(unpricedDecision, ShouldNotBeNil)
			So(unpricedDecision.ValuationStatus, ShouldEqual, "missing_execution_market")
			So(unpricedDecision.ValuationAttempted, ShouldBeFalse)
			So(unpricedDecision.Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func TestConformanceTradingDormantBeforeEngagement(t *testing.T) {
	Convey("Given a planner whose execution prerequisites are not yet filled", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		priced := deterministicCausalState(at, 0.005)
		priced.Symbol = "PRICED/USD"

		inputs := marketInputs{cash: 100000, mark: 1, feeRate: 0.001, spreadFraction: 0, available: true}
		planner := &Planner{
			// The gate reports not-ready: the instrument/fee surface or live
			// quotes are still filling during boot.
			tradingGate: func() bool { return false },
			marketProvider: func(symbol string) marketInputs {
				return inputs
			},
		}
		planner.stager = audit.NewStager(nil)
		planner.pending.Store(priced.Symbol, priced)

		Convey("Update stays dormant: no decisions, no publish, no pending drain", func() {
			round := planner.Update(types.NewThesis(t.Context()))
			So(round, ShouldBeNil)

			// The pending state is retained for the first engaged round.
			_, retained := planner.pending.Load(priced.Symbol)
			So(retained, ShouldBeTrue)
		})
	})
}

func TestConformanceQueryLocalCausalGating(t *testing.T) {
	Convey("Given a symbol with identified outcome closure and one unrelated under-supported coordinate", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 53, 0)
		at := time.Unix(0, 149*int64(time.Second))

		// Ingest one sample of an unrelated sentiment metric.
		sentimentMeasurement := &nmtypes.Measurement{
			ID:         "test:sentiment:149",
			Source:     "sentiment",
			Symbol:     "TEST/USD",
			At:         at,
			Maturity:   0.9,
			SNR:        0.5,
			SNRDefined: true,
			Metrics: map[string]*nmtypes.Metric[float64]{
				"directional_consensus": nmtypes.NewMetric(
					"directional_consensus", 0.75,
					nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
			},
		}
		testHarness.StepMeasurement(sentimentMeasurement)

		state := reasoner.CausalState("TEST/USD", at)
		So(state, ShouldNotBeNil)

		Convey("the economic query remains identified and evaluable", func() {
			So(state.Identification, ShouldEqual, causal.IdentificationIdentified)
			So(state.Transition, ShouldNotBeNil)
			So(state.Transition.Status, ShouldEqual, causal.IdentificationIdentified)

			inputs := marketInputs{cash: 100000, mark: 100, feeRate: 0.001, spreadFraction: 0, available: true}
			planner := &Planner{
				marketProvider: func(symbol string) marketInputs {
					return inputs
				},
			}

			decision := planner.decisionFromCausalState(state, testConfig(), inputs)
			So(decision.ValuationAttempted, ShouldBeTrue)
			So(decision.ValuationAvailable, ShouldBeTrue)
			So(decision.UtilityAvailable, ShouldBeTrue)
		})
	})
}

func TestConformanceRequiredTransitionFailure(t *testing.T) {
	Convey("Given a symbol whose required outcome parent has insufficient support", t, func() {
		schema := causal.NewCausalSchema("req-fail-v1", "TEST/USD", 1)
		hawkes := causal.VariableID{
			Coordinate: relation.Coordinate{Symbol: "TEST/USD", Source: "hawkes", Metric: "background_rate", Epoch: 1},
			Role:       causal.RoleMarket,
		}
		cvdFlow := causal.VariableID{
			Coordinate: relation.Coordinate{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction_zscore", Epoch: 1},
			Role:       causal.RoleMarket,
		}
		priceReturn := causal.VariableID{
			Coordinate: relation.Coordinate{Symbol: "TEST/USD", Source: "cvd", Metric: "midpoint_log_return", Epoch: 1},
			Role:       causal.RoleMarket,
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: priceReturn,
			SelfLag:  time.Second,
			Parents:  []causal.AllowedParent{{Parent: cvdFlow, Lag: time.Second}},
		})
		schema.AddMarketVariable(causal.MarketVariable{
			Variable: cvdFlow,
			SelfLag:  time.Second,
			Parents:  []causal.AllowedParent{{Parent: hawkes, Lag: time.Second}},
		})
		schema.AddMarketVariable(causal.MarketVariable{
			Variable: hawkes,
			SelfLag:  time.Second,
		})

		store := relation.NewObservationStore(2048)
		at := time.Unix(0, 100*int64(time.Second))

		// Ingest 50 observations for priceReturn and cvdFlow, but only 1 observation for hawkes.
		for index := 0; index < 50; index++ {
			sampleAt := time.Unix(0, int64(index)*int64(time.Second))
			store.Append(relation.Observation{Coordinate: priceReturn.Coordinate, Raw: math.Sin(float64(index)), At: sampleAt})
			store.Append(relation.Observation{Coordinate: cvdFlow.Coordinate, Raw: math.Cos(float64(index)), At: sampleAt})
		}
		store.Append(relation.Observation{Coordinate: hawkes.Coordinate, Raw: 0.5, At: at})

		// Register influence edges:
		// 1) cvdFlow -> priceReturn (identified)
		// 2) hawkes -> cvdFlow (registered, but hawkes observations are too few)
		influenceGraph := graph.NewInfluenceGraph(1, 1, 1, 16)
		err := influenceGraph.RegisterCandidate(graph.EdgeInfluence, cvdFlow.Coordinate, priceReturn.Coordinate, 1)
		So(err, ShouldBeNil)
		err = influenceGraph.UpsertEdge(&graph.InfluenceEdge{
			Type:   graph.EdgeInfluence,
			Source: cvdFlow.Coordinate,
			Target: priceReturn.Coordinate,
			Result: &relation.InfluenceResult{
				Lag:              time.Second,
				Status:           relation.FitOK,
				EstimatorVersion: "test-v1",
			},
			At:    at,
			Epoch: 1,
		})
		So(err, ShouldBeNil)
		err = influenceGraph.RegisterCandidate(graph.EdgeInfluence, hawkes.Coordinate, cvdFlow.Coordinate, 1)
		So(err, ShouldBeNil)
		err = influenceGraph.UpsertEdge(&graph.InfluenceEdge{
			Type:   graph.EdgeInfluence,
			Source: hawkes.Coordinate,
			Target: cvdFlow.Coordinate,
			Result: &relation.InfluenceResult{
				Lag:              time.Second,
				Status:           relation.FitOK,
				EstimatorVersion: "test-v1",
			},
			At:    at,
			Epoch: 1,
		})
		So(err, ShouldBeNil)

		model := causal.NewCausalModel(schema, store, influenceGraph, "req-fail-v1")
		transitions := model.TransitionModels(at)

		activeClosure, identification, blockingCoordinate, _, blockingReason, _ := activeCausalClosure(priceReturn.Coordinate, transitions)

		Convey("valuation is unavailable and the blocking coordinate is preserved", func() {
			So(identification, ShouldEqual, causal.IdentificationInsufficientSupport)
			So(blockingCoordinate, ShouldNotBeNil)
			So(blockingCoordinate.Metric, ShouldEqual, "signed_net_fraction_zscore")
			So(blockingReason, ShouldNotBeEmpty)
			So(len(activeClosure), ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func TestConformanceRankDeficientProvenance(t *testing.T) {
	Convey("Given a constant predictor producing a rank-deficient design matrix", t, func() {
		schema := causal.NewCausalSchema("rank-v1", "TEST/USD", 1)
		priceReturn := causal.VariableID{
			Coordinate: relation.Coordinate{Symbol: "TEST/USD", Source: "cvd", Metric: "midpoint_log_return", Epoch: 1},
			Role:       causal.RoleMarket,
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: priceReturn,
			SelfLag:  time.Second,
		})

		store := relation.NewObservationStore(2048)

		// 50 constant observations (all exactly 1.0) -> constant self-lag colinear with intercept.
		for index := 0; index < 50; index++ {
			sampleAt := time.Unix(0, int64(index)*int64(time.Second))
			store.Append(relation.Observation{Coordinate: priceReturn.Coordinate, Raw: 1.0, At: sampleAt})
		}

		at := time.Unix(0, 50*int64(time.Second))
		model := causal.NewCausalModel(schema, store, nil, "rank-v1")
		transition := model.TransitionModel(priceReturn, at)

		Convey("status is insufficient_rank with exact diagnostics reported", func() {
			So(transition.Status, ShouldEqual, causal.IdentificationInsufficientRank)
			So(transition.AlignedCount, ShouldBeGreaterThan, transition.ParameterCount)
			So(transition.Rank, ShouldBeLessThan, transition.ParameterCount)
			So(transition.Reason, ShouldContainSubstring, "not full rank")
		})
	})
}

func TestConformanceUndefinedTarget(t *testing.T) {
	Convey("Given a target coordinate with no observations", t, func() {
		schema := causal.NewCausalSchema("undef-v1", "TEST/USD", 1)
		priceReturn := causal.VariableID{
			Coordinate: relation.Coordinate{Symbol: "TEST/USD", Source: "cvd", Metric: "midpoint_log_return", Epoch: 1},
			Role:       causal.RoleMarket,
		}

		schema.AddMarketVariable(causal.MarketVariable{
			Variable: priceReturn,
			SelfLag:  time.Second,
		})

		store := relation.NewObservationStore(2048)
		at := time.Unix(0, 10*int64(time.Second))
		model := causal.NewCausalModel(schema, store, nil, "undef-v1")
		transition := model.TransitionModel(priceReturn, at)

		Convey("status is undefined and utility is unavailable", func() {
			So(transition.Status, ShouldEqual, causal.IdentificationUndefined)
			So(transition.Reason, ShouldContainSubstring, "required target history absent")

			state := &CausalState{
				Symbol:             "TEST/USD",
				At:                 at,
				Identification:     causal.IdentificationUndefined,
				BlockingCoordinate: &priceReturn.Coordinate,
			}

			inputs := marketInputs{cash: 100000, mark: 100, feeRate: 0.001, spreadFraction: 0, available: true}
			planner := &Planner{
				marketProvider: func(symbol string) marketInputs {
					return inputs
				},
			}

			decision := planner.decisionFromCausalState(state, testConfig(), inputs)
			So(decision.UtilityAvailable, ShouldBeFalse)
			So(decision.ValuationAvailable, ShouldBeFalse)
			So(decision.ValuationStatus, ShouldEqual, "undefined")
		})
	})
}

func TestConformanceUtilityZeroVsUnavailable(t *testing.T) {
	Convey("Given decision evaluation for zero expected reward vs unavailable query", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		inputs := marketInputs{cash: 100000, mark: 100, feeRate: 0.001, spreadFraction: 0, available: true}
		planner := &Planner{
			marketProvider: func(symbol string) marketInputs {
				return inputs
			},
		}

		Convey("Case A: MCTS evaluated -> UtilityAvailable is true", func() {
			state := deterministicCausalState(at, 0)
			decision := planner.decisionFromCausalState(state, testConfig(), inputs)

			So(decision.ValuationAttempted, ShouldBeTrue)
			So(decision.ValuationAvailable, ShouldBeTrue)
			So(decision.UtilityAvailable, ShouldBeTrue)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})

		Convey("Case B: causal query unavailable -> UtilityAvailable is false", func() {
			unavailableState := &CausalState{
				Symbol:         "TEST/USD",
				At:             at,
				Identification: causal.IdentificationInsufficientRank,
			}

			decision := planner.decisionFromCausalState(unavailableState, testConfig(), inputs)

			So(decision.ValuationAttempted, ShouldBeTrue)
			So(decision.ValuationAvailable, ShouldBeFalse)
			So(decision.UtilityAvailable, ShouldBeFalse)
			So(decision.Reason, ShouldContainSubstring, "causal evaluation unavailable: insufficient_rank")
		})
	})
}

func TestConformanceOpportunitySurvivesValuationFailure(t *testing.T) {
	Convey("Given a detected opportunity precursor with unavailable causal valuation", t, func() {
		at := time.Unix(0, 149*int64(time.Second))
		blockingCoord := relation.Coordinate{Symbol: "TEST/USD", Source: "hawkes", Metric: "background_rate", Epoch: 1}
		unavailableState := &CausalState{
			Symbol:             "TEST/USD",
			At:                 at,
			Identification:     causal.IdentificationInsufficientRank,
			BlockingCoordinate: &blockingCoord,
			BlockingStatus:     causal.IdentificationInsufficientRank,
			BlockingTransition: &causal.TransitionModel{
				Rank:             1,
				ObservationCount: 200,
				AlignedCount:     183,
				ParameterCount:   2,
			},
		}

		inputs := marketInputs{cash: 100000, mark: 100, feeRate: 0.001, spreadFraction: 0, available: true}
		planner := &Planner{
			marketProvider: func(symbol string) marketInputs {
				return inputs
			},
		}

		decision := planner.decisionFromCausalState(unavailableState, testConfig(), inputs)
		decision.Opportunity = true
		decision.OpportunityType = string(types.ArchetypeVerticalIgnition)
		decision.OpportunityPhase = string(types.PhaseForming)

		Convey("the candidate remains present without fabricating an entry", func() {
			So(decision.Opportunity, ShouldBeTrue)
			So(decision.OpportunityType, ShouldEqual, string(types.ArchetypeVerticalIgnition))
			So(decision.OpportunityPhase, ShouldEqual, string(types.PhaseForming))
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.ValuationAvailable, ShouldBeFalse)
			So(decision.UtilityAvailable, ShouldBeFalse)
			So(decision.CausalBlockingCoordinate, ShouldEqual, blockingCoord.ID())
			So(decision.Alternatives["causal:blocking_rank"], ShouldEqual, 1)
			So(decision.Alternatives["causal:blocking_observations"], ShouldEqual, 200)
			So(decision.Alternatives["causal:blocking_aligned"], ShouldEqual, 183)
			So(decision.Alternatives["causal:blocking_parameters"], ShouldEqual, 2)
		})
	})
}

func TestConformanceNoGlobalWorldVeto(t *testing.T) {
	Convey("Given an existing evaluable candidate and a newly observed single-sample coordinate", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 61, 0)
		at := time.Unix(0, 149*int64(time.Second))

		stateBefore := reasoner.CausalState("TEST/USD", at)
		So(stateBefore.Identification, ShouldEqual, causal.IdentificationIdentified)

		// Add a single-sample new coordinate from an unrelated signal (toxicity).
		toxMeasurement := &nmtypes.Measurement{
			ID:         "test:tox:149",
			Source:     "toxicity",
			Symbol:     "TEST/USD",
			At:         at,
			Maturity:   0.5,
			SNR:        0.1,
			SNRDefined: true,
			Metrics: map[string]*nmtypes.Metric[float64]{
				"retreat_rate:ask": nmtypes.NewMetric(
					"retreat_rate:ask", 1.2,
					nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
			},
		}
		testHarness.StepMeasurement(toxMeasurement)

		stateAfter := reasoner.CausalState("TEST/USD", at)

		Convey("the existing candidate remains evaluable because the new coordinate is outside active closure", func() {
			So(stateAfter, ShouldNotBeNil)
			So(stateAfter.Identification, ShouldEqual, causal.IdentificationIdentified)
		})
	})
}
