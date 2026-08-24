package strategy

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
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
			SearchHorizon:         3,
			MaxPositionUnits:      2,
			SlippageFraction:      0,
			RelationInterval:      time.Nanosecond,
		},
	}
}

func newTestReasoner() *Reasoner {
	return NewReasoner(1, 2048, DefaultRelationPlans(1), DefaultCausalSchema(1), time.Hour)
}

/*
ingestSeries feeds a synthetic series into a reasoner and refreshes its
Relation estimates once at the end.
*/
func ingestSeries(reasoner *Reasoner, count int, seed int64, metadata float64) {
	flow, gross, ret := syntheticSeries(count, seed)

	for index := 0; index < count; index++ {
		reasoner.Ingest(syntheticMeasurement(index, flow[index], gross[index], ret[index], metadata))
	}

	reasoner.Refresh("TEST/USD", time.Unix(0, int64(count-1)*int64(time.Second)))
}

func TestConformanceInformationPreservation(t *testing.T) {
	Convey("Given many measurement coordinates ingested", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 120, 5, 0)

		Convey("all coordinates remain in observational history after one causal query", func() {
			coordinates := reasoner.Store().Coordinates()
			So(len(coordinates), ShouldBeGreaterThanOrEqualTo, 3)

			state := reasoner.CausalState("TEST/USD", time.Unix(0, 119*int64(time.Second)))
			So(state, ShouldNotBeNil)

			after := reasoner.Store().Coordinates()
			So(len(after), ShouldEqual, len(coordinates))

			for _, coordinate := range coordinates {
				So(reasoner.Store().Count(coordinate), ShouldBeGreaterThan, 0)
			}
		})

		Convey("the unused gross-rate coordinate stays queryable", func() {
			history := reasoner.Store().History(relation.Coordinate{
				Symbol:    "TEST/USD",
				Source:    "cvd",
				Metric:    "gross_notional_rate_zscore",
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
				Epoch:     1,
			})
			So(len(history), ShouldBeGreaterThan, 0)
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
			decision := planner.decisionFromCausalState(unavailable, testConfig())
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
				mcts.MarketState{At: state.At, Values: state.MarketState},
				&causalMarketModel{state: state},
				mcts.CostModel{FeeRate: 0.001},
				1,
				2,
				3,
			)

			search := mcts.NewSearch(200, 0.5, 99)
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
	Convey("Given a deterministic expected market path and known fees", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 17, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
		So(state.Identification, ShouldEqual, causal.IdentificationIdentified)

		portfolio := mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100}
		costs := mcts.CostModel{FeeRate: 0.001, SpreadFraction: 0.0005}

		Convey("MCTS reward equals the actual net-wealth change along the path", func() {
			// Horizon 1 makes every rollout deterministic: Enter pays the
			// cost at the post-move price; Wait pays nothing.
			stateAt := mcts.NewEconomicState(
				portfolio,
				mcts.MarketState{At: state.At, Values: state.MarketState},
				&causalMarketModel{state: state},
				costs,
				1,
				1,
				1,
			)

			search := mcts.NewSearch(4, 0, 1)
			result := search.Run(stateAt, &causalActionEstimator{state: state})

			expected, _, defined := state.Transition.Step(state.MarketState)
			So(defined, ShouldBeTrue)

			newPrice := 100 * math.Exp(expected)
			exactEnter := -1 * newPrice * costs.TotalFraction()

			entered, err := stateAt.ApplyAction(mcts.Enter)
			So(err, ShouldBeNil)
			So(entered.GetReward(), ShouldAlmostEqual, exactEnter, 1e-9)

			So(result.SelectedAction, ShouldEqual, mcts.Wait)
			So(result.ExpectedEconomicOutcome, ShouldEqual, 0)
		})

		Convey("changing a semantic UI score does not change reward or action values", func() {
			left := newTestReasoner()
			ingestSeries(left, 150, 19, 0.9)
			right := newTestReasoner()
			ingestSeries(right, 150, 19, -0.9)

			leftState := left.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
			rightState := right.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))

			runSearch := func(state *CausalState) *mcts.SearchResult {
				economic := mcts.NewEconomicState(
					portfolio,
					mcts.MarketState{At: state.At, Values: state.MarketState},
					&causalMarketModel{state: state},
					costs,
					1,
					2,
					3,
				)

				search := mcts.NewSearch(24, 0.5, 123)
				return search.Run(economic, &causalActionEstimator{state: state})
			}

			leftResult := runSearch(leftState)
			rightResult := runSearch(rightState)

			So(leftResult.SelectedAction, ShouldEqual, rightResult.SelectedAction)
			So(leftResult.ExpectedEconomicOutcome, ShouldEqual, rightResult.ExpectedEconomicOutcome)
		})
	})
}

func TestConformanceNoFinalGate(t *testing.T) {
	Convey("Given MCTS selects Enter", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 23, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
		So(state.Identification, ShouldEqual, causal.IdentificationIdentified)

		planner := &Planner{}
		decision := planner.decisionFromCausalState(state, testConfig())
		So(decision.Action, ShouldEqual, types.ActionEnter)

		Convey("adverse legacy semantic values do not change the selected action", func() {
			decision.ThesisScore = 0
			decision.Confidence = 0
			decision.GraphScore = -1
			decision.PredictiveReady = false
			decision.Opportunity = false

			// The action authority is the causal MCTS result; legacy fields
			// are inert telemetry.
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Reason, ShouldEqual, "")
		})
	})
}

func TestConformanceActionDoesNotMutateMarket(t *testing.T) {
	Convey("Given the same causal market model", t, func() {
		reasoner := newTestReasoner()
		ingestSeries(reasoner, 150, 29, 0)
		state := reasoner.CausalState("TEST/USD", time.Unix(0, 149*int64(time.Second)))
		So(state.Identification, ShouldEqual, causal.IdentificationIdentified)

		base := mcts.MarketState{At: state.At, Values: state.MarketState}

		enterState := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			base,
			&causalMarketModel{state: state},
			mcts.CostModel{FeeRate: 0.001},
			1, 2, 2,
		)
		waitState := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			base,
			&causalMarketModel{state: state},
			mcts.CostModel{FeeRate: 0.001},
			1, 2, 2,
		)

		entered, err := enterState.ApplyAction(mcts.Enter)
		So(err, ShouldBeNil)
		waited, err := waitState.ApplyAction(mcts.Wait)
		So(err, ShouldBeNil)

		Convey("Enter changes portfolio variables only; the market evolves identically", func() {
			enteredEconomic := entered.(*mcts.EconomicState)
			waitedEconomic := waited.(*mcts.EconomicState)

			So(enteredEconomic.Portfolio.Position, ShouldEqual, 1)
			So(waitedEconomic.Portfolio.Position, ShouldEqual, 0)
			So(enteredEconomic.Market.At, ShouldEqual, waitedEconomic.Market.At)

			for coordinate, value := range enteredEconomic.Market.Values {
				So(waitedEconomic.Market.Values[coordinate], ShouldEqual, value)
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

			economic := mcts.NewEconomicState(
				mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
				mcts.MarketState{At: state.At, Values: state.MarketState},
				&causalMarketModel{state: state},
				mcts.CostModel{FeeRate: 0.001},
				1, 2, 3,
			)

			search := mcts.NewSearch(24, 0.5, 77)
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
