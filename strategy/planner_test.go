package strategy

import (
	"math"
	"testing"

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
			So(decision.Utility, ShouldBeGreaterThan, 0)
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

func plannerGraphThesis(t testing.TB, confidence float64) *types.Thesis {
	t.Helper()
	thesis := types.NewThesis(t.Context(), nil)
	forecast := &learning.RLSOutput{Value: 0.01, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}
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
