package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/logic/causal"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

func TestPlannerSearch(t *testing.T) {
	Convey("Given an incomplete causal artifact", t, func() {
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		decision, err := planner.search(
			types.NewThesis(t.Context(), nil),
			"BTC/USD",
			map[string]any{},
		)

		Convey("Then it should reject the malformed estimate", func() {
			So(err, ShouldNotBeNil)
			So(decision, ShouldBeNil)
		})
	})

	Convey("Given the first causal observation", t, func() {
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.80)

		decision, err := planner.search(thesis, "BTC/USD", map[string]any{
			"historyRows":    [][]float64{{1, 2, 3, 4}},
			"precision":      0.0,
			"samples":        1,
			"treatmentLevel": 3.0,
		})

		Convey("Then it should run the packaged search and retain zero precision", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.CausalPrecision, ShouldEqual, 0.0)
			So(decision.Alternatives, ShouldBeEmpty)
			So(decision.Utility, ShouldEqual, 0.0)
			So(decision.Confidence, ShouldEqual, 0.80)
			So(decision.Trace, ShouldNotBeNil)
		})
	})

	Convey("Given causal history without a named cognition class", t, func() {
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		rows := make([][]float64, 12)

		for index := range rows {
			treatment := float64(index % 2)
			rows[index] = []float64{float64(index), 0, treatment, treatment}
		}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.80)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Symbol:     "BTC/USD",
			Confidence: 0.25,
		})

		decision, err := planner.search(thesis, "BTC/USD", map[string]any{
			"historyRows":    rows,
			"precision":      0.5,
			"samples":        len(rows),
			"treatmentLevel": 1.0,
		})

		Convey("Then cognition confidence should influence allocation without blocking the estimate", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.CausalPrecision, ShouldEqual, 0.5)
			So(decision.Utility, ShouldEqual, 0.0)
		})
	})

	Convey("Given causal history whose intervention raises realized return", t, func() {
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		rows := make([][]float64, 12)

		for index := range rows {
			treatment := float64(index % 2)
			rows[index] = []float64{
				float64(index),
				math.Sin(float64(index)),
				treatment,
				treatment,
			}
		}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.80)

		decision, err := planner.search(thesis, "BTC/USD", map[string]any{
			"historyRows":    rows,
			"precision":      0.75,
			"samples":        len(rows),
			"treatmentLevel": 1.0,
		})

		Convey("Then it should select entry", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.CausalPrecision, ShouldEqual, 0.75)
			So(decision.Utility, ShouldEqual, 0.0)
		})
	})

	Convey("Given positive causal history with graph contradiction above the forecast", t, func() {
		planner := &Planner{
			mctsEngine:        plannerMCTSEngine(),
			minimumConfidence: 0.80,
		}
		rows := make([][]float64, 12)

		for index := range rows {
			treatment := float64(index % 2)
			rows[index] = []float64{
				float64(index),
				math.Sin(float64(index)),
				treatment,
				math.Sin(float64(index)) + treatment/float64(len(rows)),
			}
		}

		thesis := plannerThesisFixture(t, "BTC/USD", 0.90)
		plannerGraphRelation(
			thesis,
			"BTC/USD",
			logicgraph.RelationContradicts,
			1,
			1,
		)
		decision, err := planner.search(thesis, "BTC/USD", map[string]any{
			"historyRows":    rows,
			"precision":      0.75,
			"samples":        len(rows),
			"treatmentLevel": 1.0,
		})

		Convey("Then the packaged search should select the robust child without a local override", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Confidence, ShouldAlmostEqual, 0.90)
			So(decision.Utility, ShouldEqual, 0.0)
		})
	})
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a fully analyzed thesis whose forecast is unavailable", t, func() {
		messages := make(chan []byte, 2)
		planner := &Planner{ui: messages}
		causalSolver := causal.NewSolver(nil, nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
			Source: types.SourceResonance,
			Symbol: "BTC/USD",
		})
		thesis.Graphs.Store(marketGraphKey, logicgraph.NewGraph(thesis.At))
		Reset(func() {
			So(causalSolver.Close(), ShouldBeNil)
		})

		for _, source := range []types.SourceType{
			types.SourceCorrelation,
			types.SourceCVD,
			types.SourceDepthFlow,
			types.SourceExhaustion,
			types.SourceHawkes,
			types.SourceLeadLag,
			types.SourceLiquidity,
			types.SourcePumpDump,
			types.SourceSentiment,
			types.SourceToxicity,
			types.SourceCategories,
			types.SourceCognition,
			types.SourceManifold,
			types.SourceResonance,
			types.SourceGraph,
		} {
			thesis.Readiness.Stamp(source)
		}

		So(causalSolver.Update(thesis), ShouldBeNil)
		planner.Update(thesis)
		planner.Update(thesis)

		Convey("Then Planner should complete without inventing a decision", func() {
			So(thesis.Readiness.Planner, ShouldBeTrue)
			So(thesis.Readiness.Complete(), ShouldBeTrue)
			_, found := thesis.Decisions.Load("BTC/USD")
			So(found, ShouldBeFalse)
			So(len(messages), ShouldEqual, 0)
		})
	})

	Convey("Given a complete logic cut without causal candidates", t, func() {
		planner := &Planner{}
		thesis := types.NewThesis(t.Context(), nil)

		for _, source := range []types.SourceType{
			types.SourceCategories,
			types.SourceCognition,
			types.SourceManifold,
			types.SourceResonance,
			types.SourceCausal,
			types.SourceGraph,
		} {
			thesis.Readiness.Stamp(source)
		}

		planner.Update(thesis)

		Convey("Then Planner should close the epoch without inventing a decision", func() {
			So(thesis.StrategyDecided(), ShouldBeTrue)
		})
	})
}

func TestPlannerDecisions(t *testing.T) {
	Convey("Given a thesis with a zero-precision causal estimate", t, func() {
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.80)
		thesis.Causal.Store("BTC/USD", map[string]any{
			"historyRows":    [][]float64{{1, 2, 3, 4}},
			"precision":      0.0,
			"samples":        1,
			"treatmentLevel": 3.0,
		})

		decisions, err := planner.decisions(thesis)

		Convey("Then Planner should retain the estimate on Do Not Enter", func() {
			So(err, ShouldBeNil)
			So(decisions, ShouldHaveLength, 1)
			So(decisions[0].Action, ShouldEqual, types.ActionNothing)
			So(decisions[0].CausalPrecision, ShouldEqual, 0.0)
			So(decisions[0].Alternatives, ShouldBeEmpty)

			stored, found := thesis.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(stored.(*types.Decision).Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func TestPlannerAdmit(t *testing.T) {
	Convey("Given an Enter verdict from causal search", t, func() {
		planner := &Planner{minimumConfidence: 0.80}

		Convey("Then a forecast without a predictive distribution becomes Do Not Enter", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.80)
			stored, _ := thesis.Resonance.Load("BTC/USD")
			reading := stored.(types.ResonanceReading)
			So(reading.Forecast.SetPredictiveDistribution(0, 0, false), ShouldBeNil)

			decision, _, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Reason, ShouldContainSubstring, "not ready")
		})

		Convey("Then a forecast below the configured confidence becomes Do Not Enter", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.79)

			decision, _, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Confidence, ShouldEqual, 0.79)
			So(decision.Forecast, ShouldNotBeNil)
			So(decision.Reason, ShouldContainSubstring, "below regulated minimum")
		})

		Convey("Then a forecast at the configured confidence stays attached to Enter", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.80)
			stored, _ := thesis.Resonance.Load("BTC/USD")
			forecast := stored.(types.ResonanceReading).Forecast

			decision, _, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Forecast, ShouldEqual, forecast)
			So(decision.Confidence, ShouldAlmostEqual, 0.80)
			So(decision.Uncertainty, ShouldAlmostEqual, 0.009950166250831947, 1e-12)
		})

		Convey("Then direct graph support remains separate from forecast confidence", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.80)
			plannerGraphRelation(
				thesis,
				"BTC/USD",
				logicgraph.RelationSupports,
				1,
				0.50,
			)

			decision, evidence, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Confidence, ShouldAlmostEqual, 0.80)
			So(evidence.supports, ShouldAlmostEqual, 0.50)
		})

		Convey("Then direct graph contradiction remains separate from forecast confidence", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.90)
			plannerGraphRelation(
				thesis,
				"BTC/USD",
				logicgraph.RelationContradicts,
				1,
				0.50,
			)

			decision, evidence, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Confidence, ShouldAlmostEqual, 0.90)
			So(evidence.contradicts, ShouldAlmostEqual, 0.50)
		})

		Convey("Then cognition cannot grant uncalibrated reserve capacity", func() {
			thesis := plannerThesisFixture(t, "BTC/USD", 0.90)
			thesis.Manifold.CoherenceMag2 = 0.20
			thesis.Cognition.Store("BTC/USD", types.Cognition{
				Symbol: "BTC/USD", Confidence: 0.80,
			})

			decision, _, err := planner.admit(
				thesis,
				types.NewDecision(types.ActionEnter, "BTC/USD"),
			)

			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.AllocationClass, ShouldEqual, allocationClassNormal)
			So(decision.Opportunity, ShouldBeFalse)
			So(decision.CognitiveLead, ShouldEqual, 0.0)
		})
	})
}

func plannerMCTSEngine() *mcts.CausalMCTS {
	return mcts.NewCausalMCTS(
		NewCausalEngineAdapter(),
		math.Sqrt2,
		1,
		0,
		2,
		3,
		[]int{0, 1},
		[]int{0, 1, 2},
		false,
	)
}

func plannerThesisFixture(
	t testing.TB,
	symbol string,
	confidence float64,
) *types.Thesis {
	t.Helper()
	thesis := types.NewThesis(t.Context(), nil)
	forecast := forecastFixture(t, confidence)
	thesis.Resonance.Store(symbol, types.ResonanceReading{
		Source:   types.SourceResonance,
		Symbol:   symbol,
		At:       thesis.At,
		Forecast: forecast,
		Samples:  12,
	})
	marketGraph := logicgraph.NewGraph(thesis.At)
	marketGraph.AddNode(&logicgraph.Node{
		ID:         "res:" + symbol + ":forecast",
		Symbol:     symbol,
		Kind:       logicgraph.KindResonance,
		Value:      forecast.ExpectedReturn,
		Confidence: confidence,
		At:         thesis.At,
	})
	thesis.Graphs.Store(marketGraphKey, marketGraph)

	return thesis
}

func plannerGraphRelation(
	thesis *types.Thesis,
	symbol string,
	relation logicgraph.RelationType,
	weight float64,
	confidence float64,
) {
	stored, _ := thesis.Graphs.Load(marketGraphKey)
	marketGraph := stored.(*logicgraph.Graph)
	forecast := marketGraph.Nodes["res:"+symbol+":forecast"]
	causalID := "causal:" + symbol + ":" + string(relation)
	marketGraph.AddNode(&logicgraph.Node{
		ID:         causalID,
		Symbol:     forecast.Symbol,
		Kind:       logicgraph.KindCausal,
		Confidence: confidence,
		At:         thesis.At,
	})
	marketGraph.AddEdge(&logicgraph.Edge{
		From:       forecast.ID,
		To:         causalID,
		Relation:   relation,
		Weight:     weight,
		Confidence: confidence,
		At:         thesis.At,
	})
}

func forecastFixture(t testing.TB, confidence float64) *types.ResonanceForecast {
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

	if err := forecast.SetPredictiveDistribution(0.01, 12, true); err != nil {
		t.Fatalf("forecast distribution: %v", err)
	}

	return forecast
}

func BenchmarkPlannerDecisions(b *testing.B) {
	planner := &Planner{
		mctsEngine:        plannerMCTSEngine(),
		minimumConfidence: 1,
	}
	rows := strategyRowsFixture(-1)

	for b.Loop() {
		thesis := plannerThesisFixture(b, "BTC/USD", 0.80)
		thesis.Causal.Store("BTC/USD", map[string]any{
			"historyRows":    rows,
			"precision":      0.75,
			"samples":        len(rows),
			"treatmentLevel": 1.0,
		})
		if _, err := planner.decisions(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
