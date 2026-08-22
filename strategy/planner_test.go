package strategy

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	logicgraph "github.com/theapemachine/symm/types"
)

func TestNewPlanner(t *testing.T) {
	Convey("Given the running system configuration", t, func() {
		system.NewConfig()
		planner := NewPlanner(
			t.Context(), types.NewThesis(t.Context(), nil), nil, nil,
		)
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
	Convey("Given relation confidence below the trade-probability floor", t, func() {
		system.Cfg = system.NewConfig()
		relationConfidence := 0.4933118837060111
		newThesis := func(forecastValue float64) *types.Thesis {
			thesis := types.NewThesis(t.Context(), nil)
			symbol := types.NewSymbol("BTC/USD")
			graph := plannerGraph(
				symbol.Symbol,
				logicgraph.RelationSupports,
			)
			graph.Edges[0].Confidence = relationConfidence
			graph.Forecast = &learning.RLSOutput{
				Value:            forecastValue,
				Scale:            0.005,
				DegreesOfFreedom: 4,
				Ready:            true,
			}
			graph.ForecastHorizon = 1
			plannerOpportunityEvidence(graph, symbol.Symbol)
			symbol.Graphs.Push(graph)
			thesis.Symbols.Store(symbol.Symbol, symbol)

			return thesis
		}

		Convey("A supportive forecast should own admission", func() {
			thesis := newThesis(0.01)
			planner := &Planner{mctsEngine: plannerMCTSEngine()}

			err := planner.Update(thesis)
			decision := decisionOf(thesis, "BTC/USD")

			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.ThesisConfidence, ShouldEqual, relationConfidence)
			So(decision.Confidence, ShouldBeGreaterThan, 0.5)
			So(decision.Action, ShouldEqual, types.ActionEnter)
			So(decision.Reason, ShouldBeEmpty)
		})

		Convey("A contrary forecast should remain observably rejected", func() {
			thesis := newThesis(-0.01)
			planner := &Planner{mctsEngine: plannerMCTSEngine()}

			err := planner.Update(thesis)
			decision := decisionOf(thesis, "BTC/USD")

			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.ThesisConfidence, ShouldEqual, relationConfidence)
			So(decision.Confidence, ShouldBeLessThan, 0.5)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Reason, ShouldContainSubstring,
				"minimum confidence floor")
		})
	})

	Convey("Given a supportive graph before predictive coding has calibrated", t, func() {
		system.Cfg = system.NewConfig()
		thesis := plannerGraphThesis(t, logicgraph.RelationSupports)
		planner := &Planner{mctsEngine: plannerMCTSEngine()}
		observed := map[string]int{}
		planner.ObserveModule = func(name string, _ time.Duration) {
			observed[name]++
		}

		err := planner.Update(thesis)
		decision := decisionOf(thesis, "BTC/USD")

		Convey("It should keep structural evidence observable without committing capital", func() {
			So(err, ShouldBeNil)
			So(decision, ShouldNotBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.Forecast, ShouldBeNil)
			So(decision.Direction, ShouldEqual, float64(1))
			So(decision.ThesisScore, ShouldBeGreaterThan, 0)
			So(decision.ThesisSupport, ShouldBeGreaterThan, 0)
			So(decision.ThesisContradiction, ShouldEqual, 0)
			So(decision.GraphScore, ShouldEqual, 0)
			So(decision.PredictiveReady, ShouldBeFalse)
			So(decision.PredictiveStatus, ShouldContainSubstring, "calibrating")
			So(decision.ReserveEligible, ShouldBeFalse)
			So(decision.Opportunity, ShouldBeFalse)
			So(decision.OpportunityType, ShouldEqual, string(types.OpportunityNone))
			So(decision.Cause, ShouldBeEmpty)
			So(decision.Reason, ShouldContainSubstring, "actable opportunity")
			So(decision.Trace, ShouldBeNil)
			So(observed["planner"], ShouldEqual, 1)
			So(observed["mcts"], ShouldEqual, 1)
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
			So(decision.OpportunityType, ShouldEqual, string(types.OpportunitySuddenPump))
			So(decision.ReserveEligible, ShouldBeTrue)
			So(decision.Opportunity, ShouldBeTrue)
			So(decision.Cause, ShouldEqual, string(types.OpportunitySuddenPump))
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

	Convey("Given an entry that is no longer executable", t, func() {
		system.Cfg = system.NewConfig()
		attempts := 0
		planner := &Planner{
			desk: &broker.Desk{},
			executeEntry: func(types.Decision) error {
				attempts++

				return errnie.Err(
					errnie.NotAcceptable,
					"entry no longer crosses the current book",
					nil,
				)
			},
		}
		decision := types.NewDecision(types.ActionEnter, "RETRY/USD")

		err := planner.executeDecisions(
			[]*types.Decision{decision},
			time.Unix(1, 0).UTC(),
		)
		So(err, ShouldBeNil)
		So(attempts, ShouldEqual, 1)
		So(decision.Action, ShouldEqual, types.ActionNothing)
		So(decision.Reason, ShouldContainSubstring, "no longer executable")

		thesis := types.NewThesis(t.Context(), nil)
		err = planner.Update(thesis)

		Convey("It should not retry without a fresh graph", func() {
			So(err, ShouldBeNil)
			So(attempts, ShouldEqual, 1)
			So(decisionOf(thesis, "RETRY/USD"), ShouldBeNil)
		})
	})

	Convey("Given two fresh graphs and repeated planner wake tokens", t, func() {
		system.Cfg = system.NewConfig()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		thesis := types.NewThesis(ctx, nil)
		paused := atomic.Bool{}
		paused.Store(true)
		var planner *Planner
		planner = &Planner{
			ctx:        ctx,
			cancel:     cancel,
			status:     types.READY,
			mctsEngine: plannerMCTSEngine(),
			thesis:     thesis,
		}
		planner.work = transport.NewConsumer[*types.Symbol](planner.Name(), func() {
			if !paused.Load() {
				planner.consume()
			}
		})
		thesis.Work(types.SourcePlanner).Register(planner.work)
		plannerRounds := atomic.Int64{}
		searchRounds := atomic.Int64{}
		planner.ObserveModule = func(name string, _ time.Duration) {
			switch name {
			case "planner":
				plannerRounds.Add(1)
			case "mcts":
				searchRounds.Add(1)
			}
		}

		for _, symbolName := range []string{"BTC/USD", "ETH/USD"} {
			symbol := thesis.Symbol(symbolName)
			graph := plannerGraph(symbolName, logicgraph.RelationSupports)
			graph.Forecast = &learning.RLSOutput{
				Value: 0.01, Scale: 0.005, DegreesOfFreedom: 4, Ready: true,
			}
			graph.ForecastHorizon = 1
			plannerOpportunityEvidence(graph, symbolName)
			symbol.Graphs.Push(graph)
		}

		for index := 0; index < 8; index++ {
			thesis.ScheduleWork(types.SourcePlanner, nil)
		}

		paused.Store(false)
		planner.consume()
		waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
		defer waitCancel()
		err := thesis.WaitForQuiescence(waitCtx)

		Convey("It should search the cross-section exactly once", func() {
			So(err, ShouldBeNil)
			So(plannerRounds.Load(), ShouldEqual, int64(1))
			So(searchRounds.Load(), ShouldEqual, int64(1))
			So(decisionOf(thesis, "BTC/USD"), ShouldNotBeNil)
			So(decisionOf(thesis, "ETH/USD"), ShouldNotBeNil)

			thesis.ScheduleWork(types.SourcePlanner, nil)
			So(thesis.WaitForQuiescence(waitCtx), ShouldBeNil)
			So(searchRounds.Load(), ShouldEqual, int64(1))
			So(decisionOf(thesis, "BTC/USD"), ShouldBeNil)
			So(decisionOf(thesis, "ETH/USD"), ShouldBeNil)
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

	for candidate := range symbol.MarketGraphs(
		symbol.GraphConsumers[types.GraphConsumerPlanner],
	) {
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
	plannerOpportunityEvidence(graph, symbol.Symbol)
	symbol.Graphs.Push(graph)

	return thesis
}

func plannerOpportunityEvidence(graph *logicgraph.Graph, symbol string) {
	graph.TaskSkill = 1.05
	graph.TaskSkillReady = true
	graph.AddNode(&logicgraph.Node{
		ID:     "meas:" + graph.DecisionTarget + ":pumpdump:rvol",
		Symbol: symbol, Source: string(types.SourcePumpDump),
		Metric: types.MetricRVOL, Kind: logicgraph.KindMeasurement,
		Value: 1, Confidence: 1, Maturity: 1, At: graph.At,
		Metadata: map[string]any{"hypothesis_separation": 1.0},
	})
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
		Weight: 0.5, Confidence: 0.8, At: graph.At,
	})
	return graph
}

func decisionOf(thesis *types.Thesis, symbol string) *types.Decision {
	stored, found := thesis.Symbols.Load(symbol)

	if !found {
		return nil
	}

	var decision types.Decision

	symbolState := stored.(*types.Symbol)

	for candidate := range symbolState.Decisions.Drain(symbolState.DecisionConsumers[0], func(types.Decision) bool {
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
			plannerReadyGraphThesis(b, logicgraph.RelationSupports),
		); err != nil {
			b.Fatal(err)
		}
	}
}
