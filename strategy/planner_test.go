package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/broker"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewPlanner(t *testing.T) {
	Convey("Given the running system configuration", t, func() {
		system.NewConfig()
		planner := NewPlanner(t.Context(), nil, nil, nil, nil)
		defer planner.Close()

		Convey("It should initialize the reusable causal graph search", func() {
			So(planner.Status(), ShouldEqual, types.READY)
			So(planner.mctsEngine, ShouldNotBeNil)
		})
	})
}

func TestPlannerHasCapacity(t *testing.T) {
	Convey("Given a planner without a desk", t, func() {
		planner := &Planner{}

		Convey("It should treat missing broker state as open capacity", func() {
			So(planner.HasCapacity(), ShouldBeTrue)
			So(planner.Holding("BTC/USD"), ShouldBeFalse)
			So((*Planner)(nil).HasCapacity(), ShouldBeTrue)
		})
	})
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a complete graph with a regulated positive forecast", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumConfidence = 0.8
		thesis := plannerGraphThesis(t, 0.9)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should select entry from the final graph", func() {
			So(err, ShouldBeNil)
			stored, _ := thesis.Symbols.Load("BTC/USD")
			symbol := stored.(*types.Symbol)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Forecast, ShouldNotBeNil)
			So(decision.Confidence, ShouldBeGreaterThan, system.Cfg.Planner.MinimumConfidence)
			So(decision.Utility, ShouldEqual, 0)
			So(decision.GraphScore, ShouldBeGreaterThan, 0)
			So(decision.Trace, ShouldNotBeNil)
			So(decision.Trace.MCTS.Iterations, ShouldEqual, system.Cfg.Planner.MCTSIterations)
			So(decision.Trace.MCTS.Branches, ShouldHaveLength, 1)
			So(decision.Trace.MCTS.Branches[0].Action,
				ShouldEqual, "res:BTC/USD:forecast")
			So(decision.Trace.MCTS.Branches[0].Visits, ShouldBeGreaterThan, 0)
			So(decision.Trace.MCTS.RecommendedAction,
				ShouldEqual, "res:BTC/USD:forecast")
		})
	})

	Convey("Given a retained entry candidate and no newly completed graph", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("RETRY/USD")
		candidate := types.NewDecision(types.ActionEnter, "RETRY/USD")
		candidate.Cause = "opportunity_entry"
		planner := &Planner{
			mctsEngine: plannerMCTSEngine(),
			candidates: map[string]*types.Decision{
				candidate.Symbol: candidate,
			},
		}

		err := planner.Update(thesis)

		Convey("It should re-price the candidate instead of waiting for another graph", func() {
			So(err, ShouldBeNil)
			stored := decisionOf(thesis, "RETRY/USD")
			So(stored, ShouldNotBeNil)
			So(stored.Action, ShouldEqual, types.ActionEnter)
			So(stored.Cause, ShouldEqual, "opportunity_entry")
			So(planner.candidates, ShouldContainKey, "RETRY/USD")
		})
	})

	Convey("Given contradicting graph evidence under the cold-start floor", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graphValue.(*logicgraph.Graph).Edges[0].Relation = logicgraph.RelationContradicts
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should refuse entry when signed graph evidence is against the long", func() {
			So(err, ShouldBeNil)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.GraphScore, ShouldBeLessThan, 0)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})

	Convey("Given the same positive forecast with contradicting causal evidence", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumGraphScore = 0
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graphValue.(*logicgraph.Graph).Edges[0].Relation = logicgraph.RelationContradicts
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should assign a negative dimensionless evidence score", func() {
			So(err, ShouldBeNil)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Utility, ShouldEqual, 0)
			So(decision.GraphScore, ShouldBeLessThan, 0)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})

	Convey("Given a positive forecast whose posterior direction is below the regulated confidence", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumConfidence = 0.8
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graph := graphValue.(*logicgraph.Graph)
		graph.Forecast.Scale = 1
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should retain confidence as evidence without making it a fixed veto", func() {
			So(err, ShouldBeNil)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.ExpectedReturn, ShouldNotBeNil)
			So(decision.ExpectedReturn.Float64(), ShouldAlmostEqual,
				math.Expm1(decision.PerspectiveReturn), 1e-12)
			So(decision.GraphScore, ShouldBeGreaterThan,
				decision.AdmissionGraphThreshold)
		})
	})

	Convey("Given a negative predictive forecast alongside positive causal return evidence", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graph := graphValue.(*logicgraph.Graph)
		graph.Forecast.Value = -0.001
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should size only from priced return sources, not the direction lean", func() {
			So(err, ShouldBeNil)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.PerspectiveSources, ShouldHaveLength, 1)
			So(decision.PerspectiveSources[0].Source, ShouldEqual, "causal")
			So(decision.ExpectedReturn, ShouldNotBeNil)
			So(decision.ExpectedReturn.Float64(), ShouldAlmostEqual,
				math.Expm1(decision.PerspectiveReturn), 1e-12)
		})
	})

	Convey("Given a graph forecast that is not ready", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumConfidence = 0.95
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graphValue.(*logicgraph.Graph).Forecast.Ready = false
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should not create an entry", func() {
			So(err, ShouldBeNil)
			_, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeFalse)
		})
	})

	Convey("Given a positive graph score before executable economics", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumUtility = 0.02
		thesis := plannerGraphThesis(t, 0.9)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should defer the net utility threshold to allocation", func() {
			So(err, ShouldBeNil)
			stored, _ := thesis.Symbols.Load("BTC/USD")
			symbol := stored.(*types.Symbol)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Utility, ShouldEqual, 0)
			So(decision.GraphScore, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given two ready symbols presented in opposite map-insertion orders", t, func() {
		system.Cfg = system.NewConfig()
		firstOrder := plannerMultiGraphThesis(t, []string{"AAA/USD", "ZZZ/USD"}, 0.9)
		secondOrder := plannerMultiGraphThesis(t, []string{"ZZZ/USD", "AAA/USD"}, 0.9)
		firstPlanner := &Planner{mctsEngine: plannerMCTSEngine()}
		secondPlanner := &Planner{mctsEngine: plannerMCTSEngine()}

		So(firstPlanner.Update(firstOrder), ShouldBeNil)
		So(secondPlanner.Update(secondOrder), ShouldBeNil)

		Convey("It should evaluate every ready symbol in the same round", func() {
			So(decisionAction(firstOrder, "AAA/USD"), ShouldEqual, types.ActionEnter)
			So(decisionAction(firstOrder, "ZZZ/USD"), ShouldEqual, types.ActionEnter)
			So(decisionAction(secondOrder, "AAA/USD"), ShouldEqual, types.ActionEnter)
			So(decisionAction(secondOrder, "ZZZ/USD"), ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given a ready field mixed with a graph that is not searchable", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerMultiGraphThesis(t, []string{"READY/USD", "STALE/USD"}, 0.9)
		stored, _ := thesis.Symbols.Load("STALE/USD")
		graphValue, _ := stored.(*types.Symbol).Graphs.Load("market_graph")
		graphValue.(*logicgraph.Graph).Forecast.Ready = false
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should decide only the ready symbol and leave the other unevaluated", func() {
			So(err, ShouldBeNil)
			So(decisionAction(thesis, "READY/USD"), ShouldEqual, types.ActionEnter)
			_, found := stored.(*types.Symbol).Decisions.Load("STALE/USD")
			So(found, ShouldBeFalse)
		})
	})

	Convey("Given many ready symbols and a one-slot admission after search", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerMultiGraphThesis(t, []string{
			"WEAK/USD", "MID/USD", "STRONG/USD",
		}, 0.9)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		So(planner.Update(thesis), ShouldBeNil)
		decisions := []*types.Decision{
			decisionOf(thesis, "WEAK/USD"),
			decisionOf(thesis, "MID/USD"),
			decisionOf(thesis, "STRONG/USD"),
		}
		decisions[0].Utility = 0.02
		decisions[1].Utility = 0.05
		decisions[2].Utility = 0.11

		Convey("It should keep only the best fill after the planner has seen the whole field", func() {
			admitBest(decisions, 1, 0, nil)
			So(decisions[0].Action, ShouldEqual, types.ActionNothing)
			So(decisions[1].Action, ShouldEqual, types.ActionNothing)
			So(decisions[2].Action, ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given two winners whose first fill is no longer executable", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerMultiGraphThesis(t, []string{"AAA/USD", "ZZZ/USD"}, 0.9)
		executed := make([]string, 0, 2)
		planner := &Planner{
			mctsEngine: plannerMCTSEngine(),
			desk:       &broker.Desk{},
			executeEntry: func(decision types.Decision) error {
				executed = append(executed, decision.Symbol)

				if decision.Symbol == "AAA/USD" {
					return errnie.Err(
						errnie.NotAcceptable,
						"desk: entry is no longer executable",
						nil,
					)
				}

				return nil
			},
		}

		err := planner.Update(thesis)

		Convey("It should skip the thin book and still submit the remaining winner", func() {
			So(err, ShouldBeNil)
			So(executed, ShouldContain, "AAA/USD")
			So(executed, ShouldContain, "ZZZ/USD")
			So(decisionAction(thesis, "AAA/USD"), ShouldEqual, types.ActionNothing)
			So(decisionOf(thesis, "AAA/USD").Reason, ShouldContainSubstring,
				"entry is no longer executable")
			So(decisionAction(thesis, "ZZZ/USD"), ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given a structurally accepted candidate whose first quote is not executable", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, 0.9)
		attempts := 0
		planner := &Planner{
			mctsEngine: plannerMCTSEngine(),
			desk:       &broker.Desk{},
			executeEntry: func(types.Decision) error {
				attempts++

				if attempts == 1 {
					return errnie.Err(
						errnie.NotAcceptable,
						"desk: entry is no longer executable",
						nil,
					)
				}

				return nil
			},
		}

		firstErr := planner.Update(thesis)
		secondErr := planner.Update(thesis)

		Convey("It should retry the retained edge without requiring another graph", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(attempts, ShouldEqual, 2)
			So(planner.candidates, ShouldNotContainKey, "BTC/USD")
		})
	})

	Convey("Given two winners whose first fill fails validation", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerMultiGraphThesis(t, []string{"AAA/USD", "ZZZ/USD"}, 0.9)
		planner := &Planner{
			mctsEngine: plannerMCTSEngine(),
			desk:       &broker.Desk{},
			executeEntry: func(decision types.Decision) error {
				if decision.Symbol == "AAA/USD" {
					return errnie.Err(
						errnie.Validation,
						"desk: quantity, forecast, price, and strategy stoploss required for entry",
						nil,
					)
				}

				return nil
			},
		}

		err := planner.Update(thesis)

		Convey("It should still fail the tick on a broken entry payload", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "planner: execute AAA/USD")
		})
	})
}

func TestDecisionTrace(t *testing.T) {
	Convey("Given a graph search with supporting and contradicting evidence", t, func() {
		at := thesisTime()
		graph := logicgraph.NewGraph(at)
		graph.AddNode(&logicgraph.Node{ID: "root-a", Confidence: 1})
		graph.AddNode(&logicgraph.Node{ID: "root-b", Confidence: 1})
		graph.AddNode(&logicgraph.Node{ID: "leaf-a", Confidence: 1})
		graph.AddEdge(&logicgraph.Edge{
			From:       "root-a",
			To:         "leaf-a",
			Relation:   logicgraph.RelationSupports,
			Weight:     0.25,
			Confidence: 0.5,
		})
		graph.AddEdge(&logicgraph.Edge{
			From:       "root-b",
			To:         "leaf-a",
			Relation:   logicgraph.RelationContradicts,
			Weight:     0.75,
			Confidence: 0.2,
		})
		root := &mcts.Node{Children: []*mcts.Node{
			{Action: 1, Visits: 3, TotalReward: 6},
			{Action: 0, Visits: 5, TotalReward: 5},
		}}

		trace := decisionTrace(graph, root, 0, 50)

		Convey("It should report graph mass and root labels from the actual search", func() {
			So(trace.GraphSupports, ShouldEqual, 0.125)
			So(trace.GraphContradicts, ShouldEqual, 0.15000000000000002)
			So(trace.MCTS.Iterations, ShouldEqual, 50)
			So(trace.MCTS.RecommendedAction, ShouldEqual, "root-a")
			So(trace.MCTS.Branches[0].Action, ShouldEqual, "root-a")
			So(trace.MCTS.Branches[0].MeanReward, ShouldEqual, 1)
			So(trace.MCTS.Branches[1].Action, ShouldEqual, "root-b")
			So(trace.MCTS.Branches[1].MeanReward, ShouldEqual, 2)
		})
	})
}

func TestGraphEvidenceMass(t *testing.T) {
	Convey("Given reachable and disconnected evidence around a forecast root", t, func() {
		graph := logicgraph.NewGraph(thesisTime())
		forecast := "res:BTC/USD:forecast"
		graph.AddNode(&logicgraph.Node{
			ID: forecast, Symbol: "BTC/USD", Kind: logicgraph.KindResonance,
		})
		graph.AddNode(&logicgraph.Node{ID: "causal"})
		graph.AddNode(&logicgraph.Node{ID: "disconnected-source"})
		graph.AddNode(&logicgraph.Node{ID: "disconnected-target"})
		graph.AddEdge(&logicgraph.Edge{
			From: forecast, To: "causal", Relation: logicgraph.RelationSupports,
			Weight: 0.4, Confidence: 0.5,
		})
		graph.AddEdge(&logicgraph.Edge{
			From: "disconnected-source", To: "disconnected-target",
			Relation: logicgraph.RelationContradicts, Weight: 0.8, Confidence: 0.5,
		})

		supports, contradicts := graphEvidenceMass(graph)

		Convey("It should report evidence across independent graph components", func() {
			So(supports, ShouldEqual, 0.2)
			So(contradicts, ShouldEqual, 0.4)
		})
	})
}

func plannerMCTSEngine() *mcts.CausalMCTS {
	return mcts.NewCausalMCTS(
		mcts.DefaultCausalEngine{},
		math.Sqrt2,
		1,
		len(mcts.GraphFeatureColumns)+1,
		mcts.GraphTreatmentColumn,
		mcts.GraphTargetColumn,
		mcts.GraphControlColumns,
		mcts.GraphFeatureColumns,
		false,
	)
}

func thesisTime() time.Time {
	return time.Unix(1, 0).UTC()
}

func plannerGraphThesis(t testing.TB, confidence float64) *types.Thesis {
	t.Helper()

	return plannerMultiGraphThesis(t, []string{"BTC/USD"}, confidence)
}

func plannerMultiGraphThesis(
	t testing.TB,
	symbols []string,
	confidence float64,
) *types.Thesis {
	t.Helper()
	thesis := types.NewThesis(t.Context(), nil)

	for _, symbolName := range symbols {
		forecast := &learning.RLSOutput{
			Value: 0.01, Ready: true, Scale: 0.001, DegreesOfFreedom: 1,
		}
		graph := logicgraph.NewGraph(thesis.At)
		graph.Forecast = forecast
		graph.ForecastHorizon = 3
		graph.ForwardCurve = []float64{-0.002, 0.004, 0.008}
		graph.TaskSkill = 1.01
		graph.TaskSkillReady = true
		graph.AddNode(&logicgraph.Node{
			ID: "res:" + symbolName + ":forecast", Symbol: symbolName,
			Kind: logicgraph.KindResonance, Value: forecast.Value, Confidence: confidence,
		})
		graph.AddNode(&logicgraph.Node{
			ID: "causal:" + symbolName + ":doExpectation", Symbol: symbolName,
			Kind:       logicgraph.KindCausal,
			Value:      0.005,
			Confidence: confidence,
			Metadata:   map[string]any{"horizon": 1},
		})
		graph.AddEdge(&logicgraph.Edge{
			From:       "res:" + symbolName + ":forecast",
			To:         "causal:" + symbolName + ":doExpectation",
			Relation:   logicgraph.RelationSupports,
			Weight:     confidence,
			Confidence: confidence,
		})
		symbol := types.NewSymbol(symbolName, nil)
		symbol.Graphs.Store("market_graph", graph)
		thesis.Symbols.Store(symbolName, symbol)
	}

	return thesis
}

func decisionOf(thesis *types.Thesis, symbol string) *types.Decision {
	stored, found := thesis.Symbols.Load(symbol)

	if !found {
		return nil
	}

	decisionValue, found := stored.(*types.Symbol).Decisions.Load(symbol)

	if !found {
		return nil
	}

	return decisionValue.(*types.Decision)
}

func decisionAction(thesis *types.Thesis, symbol string) types.Action {
	decision := decisionOf(thesis, symbol)

	if decision == nil {
		return ""
	}

	return decision.Action
}

func BenchmarkPlannerUpdate(b *testing.B) {
	system.Cfg = system.NewConfig()
	system.Cfg.Planner.MinimumConfidence = 0.8
	planner := &Planner{mctsEngine: plannerMCTSEngine()}

	for b.Loop() {
		if err := planner.Update(plannerGraphThesis(b, 0.9)); err != nil {
			b.Fatal(err)
		}
	}
}
