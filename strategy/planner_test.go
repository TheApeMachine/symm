package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	logicgraph "github.com/theapemachine/symm/types"
)

func TestNewPlanner(t *testing.T) {
	Convey("Given the running system configuration", t, func() {
		system.NewConfig()
		planner := NewPlanner(t.Context(), nil, nil, nil)
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
		So(planner.HasCapacity(), ShouldBeTrue)
		So(planner.Holding("BTC/USD"), ShouldBeFalse)
		So((*Planner)(nil).HasCapacity(), ShouldBeTrue)
	})
}

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a supportive graph before predictive coding has calibrated", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, logicgraph.RelationSupports)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)
		decision := decisionOf(thesis, "BTC/USD")

		Convey("It should admit on structural evidence while predictive coding only enriches", func() {
			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Forecast, ShouldBeNil)
			So(decision.Direction, ShouldEqual, float64(1))
			So(decision.ThesisScore, ShouldBeGreaterThan, 0)
			So(decision.ThesisSupport, ShouldBeGreaterThan, 0)
			So(decision.ThesisContradiction, ShouldEqual, 0)
			So(decision.GraphScore, ShouldBeGreaterThan, 0)
			So(decision.PredictiveReady, ShouldBeFalse)
			So(decision.PredictiveStatus, ShouldContainSubstring, "enriching")
			So(decision.ReserveEligible, ShouldBeFalse)
			So(decision.Opportunity, ShouldBeFalse)
			So(decision.OpportunityType, ShouldEqual, string(types.VerticalIgnition))
			So(decision.Reason, ShouldBeEmpty)
			So(decision.Trace, ShouldNotBeNil)
			So(decision.Trace.Hypothesis, ShouldContainSubstring, "long_opportunity")
			So(decision.Trace.MCTS.Branches, ShouldHaveLength, 3)

			actions := make([]string, 0, 3)

			for _, branch := range decision.Trace.MCTS.Branches {
				actions = append(actions, branch.Action)
			}

			So(actions, ShouldContain, "BTC/USD:enter")
		})
	})

	Convey("Given the same supportive graph with calibrated predictive evidence", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerReadyGraphThesis(t, logicgraph.RelationSupports)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)
		decision := decisionOf(thesis, "BTC/USD")

		Convey("It should admit the transition and qualify only the sudden-pump reserve lane", func() {
			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.PredictiveReady, ShouldBeTrue)
			So(decision.PredictiveStatus, ShouldContainSubstring, "supported transition horizon")
			So(decision.TaskSkillReady, ShouldBeTrue)
			So(decision.TaskSkill, ShouldBeGreaterThanOrEqualTo, float64(1))
			So(decision.Forecast, ShouldNotBeNil)
			So(decision.ForecastHorizon, ShouldEqual, 1)
			So(decision.OpportunityType, ShouldEqual, string(types.VerticalIgnition))
			So(decision.ReserveEligible, ShouldBeTrue)
			So(decision.Opportunity, ShouldBeTrue)
			So(decision.ReserveReason, ShouldContainSubstring, "sudden-pump")
		})
	})

	Convey("Given the same evidence relation contradicting the long thesis", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, logicgraph.RelationContradicts)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)
		decision := decisionOf(thesis, "BTC/USD")

		Convey("It should publish an observable rejection rather than lose the round", func() {
			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Direction, ShouldEqual, float64(-1))
			So(decision.ThesisScore, ShouldBeLessThan, 0)
			So(decision.Reason, ShouldContainSubstring, "contradiction")
		})
	})

	Convey("Given a retained structural candidate and no newly completed graph", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("RETRY/USD")
		candidate := types.NewDecision(types.ActionEnter, "RETRY/USD")
		candidate.Cause = "vertical_ignition"
		candidate.Direction = 1
		candidate.ThesisScore = 0.6
		candidate.ThesisConfidence = 0.8
		candidate.PredictiveReady = true
		planner := &Planner{
			mctsEngine: plannerMCTSEngine(),
			candidates: map[string]*types.Decision{
				candidate.Symbol: candidate,
			},
		}

		err := planner.Update(thesis)
		stored := decisionOf(thesis, "RETRY/USD")

		Convey("It should retry the candidate on the current execution pass", func() {
			So(err, ShouldBeNil)
			So(stored, ShouldNotBeNil)
			So(stored.Action, ShouldEqual, types.ActionEnter)
			So(stored.Cause, ShouldEqual, "vertical_ignition")
			So(planner.candidates, ShouldContainKey, "RETRY/USD")
		})
	})
}

func TestDecisionTrace(t *testing.T) {
	Convey("Given one searched evidence path", t, func() {
		graph := plannerGraph("BTC/USD", logicgraph.RelationSupports)
		state, err := mcts.NewGraphState(graph)
		So(err, ShouldBeNil)
		engine := plannerMCTSEngine()
		root, action, err := engine.Search(state, 4, state.History())
		So(err, ShouldBeNil)

		trace := decisionTrace(graph, root, action, 4)
		So(trace.Hypothesis, ShouldEqual, graph.DecisionTarget)
		So(trace.GraphSupports, ShouldBeGreaterThan, 0)
		So(trace.GraphContradicts, ShouldEqual, 0)
		So(trace.ThesisBalance, ShouldEqual, float64(1))
		So(trace.MCTS.Branches, ShouldHaveLength, 1)
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

func plannerGraphThesis(
	t testing.TB,
	relation logicgraph.RelationType,
) *types.Thesis {
	t.Helper()
	thesis := types.NewThesis(t.Context(), nil)
	symbol := types.NewSymbol("BTC/USD")
	symbol.Graphs.Push(plannerGraph(symbol.Symbol, relation))
	thesis.Symbols.Store(symbol.Symbol, symbol)
	return thesis
}

func plannerReadyGraphThesis(
	t testing.TB,
	relation logicgraph.RelationType,
) *types.Thesis {
	t.Helper()
	thesis := plannerGraphThesis(t, relation)
	stored, _ := thesis.Symbols.Load("BTC/USD")
	symbol := stored.(*types.Symbol)
	var graph *logicgraph.Graph

	for candidate := range symbol.MarketGraphs(types.SourceGraph) {
		graph = candidate
	}

	graph.Forecast = &learning.RLSOutput{
		Value:            0.01,
		Scale:            0.005,
		DegreesOfFreedom: 4,
		Ready:            true,
	}
	graph.ForecastHorizon = 1
	graph.ForwardCurve = []float64{0.01}
	graph.TaskSkill = 1.05
	graph.TaskSkillReady = true
	symbol.Graphs.Push(graph)

	return thesis
}

func plannerGraph(symbol string, relation logicgraph.RelationType) *logicgraph.Graph {
	graph := logicgraph.NewGraph(time.Unix(1, 0).UTC())
	target := "hyp:" + symbol + ":long_opportunity"
	root := "cat:" + symbol + ":" + string(types.VerticalIgnition)
	graph.DecisionTarget = target
	graph.AddNode(&logicgraph.Node{
		ID: target, Symbol: symbol, Kind: logicgraph.KindHypothesis,
		Confidence: 1, At: graph.At,
	})
	graph.AddNode(&logicgraph.Node{
		ID: root, Symbol: symbol, Kind: logicgraph.KindCategory,
		Value: 0.9, Strength: 0.9, Confidence: 0.8, At: graph.At,
		Metadata: map[string]any{"type": string(types.VerticalIgnition)},
	})
	graph.AddEdge(&logicgraph.Edge{
		From: root, To: target, Relation: relation,
		Weight: 0.9, Confidence: 0.8, At: graph.At,
	})
	return graph
}

func decisionOf(thesis *types.Thesis, symbol string) *types.Decision {
	stored, found := thesis.Symbols.Load(symbol)

	if !found {
		return nil
	}

	var decision types.Decision

	for candidate := range stored.(*types.Symbol).Decisions.Drain("audit", func(types.Decision) bool {
		return true
	}) {
		decision = candidate
	}

	if decision.Symbol == "" {
		return nil
	}

	return &decision
}

func BenchmarkPlannerUpdate(b *testing.B) {
	system.Cfg = system.NewConfig()
	planner := &Planner{mctsEngine: plannerMCTSEngine()}

	for b.Loop() {
		if err := planner.Update(
			plannerGraphThesis(b, logicgraph.RelationSupports),
		); err != nil {
			b.Fatal(err)
		}
	}
}
