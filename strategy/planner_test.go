package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/mcts"
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
			So(decision.Utility, ShouldBeGreaterThan, 0)
			So(decision.Trace, ShouldNotBeNil)
			So(decision.Trace.MCTS.Iterations, ShouldEqual, system.Cfg.Planner.MCTSIterations)
			So(decision.Trace.MCTS.Branches, ShouldHaveLength, 1)
			So(decision.Trace.MCTS.Branches[0].Action, ShouldEqual, "forecast")
			So(decision.Trace.MCTS.Branches[0].Visits, ShouldBeGreaterThan, 0)
			So(decision.Trace.MCTS.RecommendedAction, ShouldEqual, "forecast")
		})
	})

	Convey("Given a positive forecast whose posterior direction is below the regulated confidence", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumConfidence = 0.8
		thesis := plannerGraphThesis(t, 0.9)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)
		graphValue, _ := symbol.Graphs.Load("market_graph")
		graphValue.(*logicgraph.Graph).Forecast.Scale = 1
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should retain the evaluated decision without admitting an entry", func() {
			So(err, ShouldBeNil)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Reason, ShouldEqual,
				"planner: forecast confidence does not clear regulated entry threshold")
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

	Convey("Given a positive graph utility below the regulated entry threshold", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Planner.MinimumUtility = 0.02
		thesis := plannerGraphThesis(t, 0.9)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}

		err := planner.Update(thesis)

		Convey("It should keep the decision out of the market", func() {
			So(err, ShouldBeNil)
			stored, _ := thesis.Symbols.Load("BTC/USD")
			symbol := stored.(*types.Symbol)
			decisionValue, found := symbol.Decisions.Load("BTC/USD")
			So(found, ShouldBeTrue)
			decision := decisionValue.(*types.Decision)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Reason, ShouldEqual, "planner: utility does not clear regulated entry threshold")
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
	thesis := types.NewThesis(t.Context(), nil)
	forecast := &learning.RLSOutput{Value: 0.01, Ready: true, Scale: 0.001, DegreesOfFreedom: 1}
	graph := logicgraph.NewGraph(thesis.At)
	graph.Forecast = forecast
	graph.AddNode(&logicgraph.Node{
		ID: "forecast", Kind: logicgraph.KindResonance,
		Value: forecast.Value, Confidence: confidence,
	})
	symbol := types.NewSymbol("BTC/USD", nil)
	symbol.Graphs.Store("market_graph", graph)
	thesis.Symbols.Store("BTC/USD", symbol)

	return thesis
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
