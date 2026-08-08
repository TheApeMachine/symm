package graph

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestUpdate(t *testing.T) {
	Convey("Given completed upstream stages with causal search still warming", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Readiness.Stamp(types.SourceCategories)
		thesis.Readiness.Stamp(types.SourceResonance)
		thesis.Readiness.Stamp(types.SourceCausal)
		thesis.Readiness.Stamp(types.SourceCognition)
		thesis.Causal.Store("BTC/USD", map[string]any{"ready": false})
		solver := NewSolver(nil, nil)

		err := solver.Update(thesis)

		Convey("It should compile and stamp the graph", func() {
			So(err, ShouldBeNil)
			So(thesis.Readiness.Graph, ShouldBeTrue)
			_, found := thesis.Graphs.Load("market_graph")
			So(found, ShouldBeTrue)
		})
	})
}

func TestAddEdge(t *testing.T) {
	Convey("Given an edge whose target is not a registered graph node", t, func() {
		graph := NewGraph(time.Unix(1, 0).UTC())
		graph.AddNode(&Node{ID: "source"})
		edge := &Edge{From: "source", To: "missing"}

		graph.AddEdge(edge)

		Convey("It should not materialize a dangling relationship", func() {
			So(graph.Edges, ShouldBeEmpty)
			So(graph.Adjacency[edge.From], ShouldBeEmpty)
		})

		Convey("It should connect the edge once both nodes exist", func() {
			graph.AddNode(&Node{ID: edge.To})
			graph.AddEdge(edge)

			So(graph.Edges, ShouldHaveLength, 1)
			So(graph.Adjacency[edge.From], ShouldResemble, []string{edge.To})
		})
	})
}

func TestInferStructuralEdges(t *testing.T) {
	Convey("Given canonical causal intervention and expectation nodes", t, func() {
		at := time.Unix(1, 0).UTC()
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

		for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
			graph.AddNode(&Node{
				ID:         "causal:" + symbol + ":intervention",
				Symbol:     symbol,
				Kind:       KindCausal,
				Value:      0.5,
				Confidence: 1,
				At:         at,
			})
			graph.AddNode(&Node{
				ID:         "causal:" + symbol + ":doExpectation",
				Symbol:     symbol,
				Kind:       KindCausal,
				Confidence: 1,
				At:         at,
			})
		}

		graph.AddNode(&Node{
			ID:         "causal:BTC/USD:not_intervention",
			Symbol:     "BTC/USD",
			Kind:       KindCausal,
			Confidence: 1,
			At:         at,
		})

		solver.inferStructuralEdges(types.NewThesis(nil), graph)

		Convey("Each intervention should condition its symbol's do-expectation", func() {
			conditionEdges := 0

			for _, edge := range graph.Edges {
				if edge.Relation == RelationConditions {
					conditionEdges++
					So(graph.Nodes[edge.From].Symbol,
						ShouldEqual, graph.Nodes[edge.To].Symbol)
				}
			}

			So(conditionEdges, ShouldEqual, 2)
		})
	})

	Convey("Given unrelated graph nodes", t, func() {
		at := time.Unix(10, 0).UTC()
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

		for index := range 256 {
			nodeID := "cat:BTC/USD:inactive:" + strconv.Itoa(index)
			graph.AddNode(&Node{
				ID:     nodeID,
				Symbol: "BTC/USD",
				Kind:   KindCategory,
				At:     at.Add(-time.Duration(index) * time.Second),
			})
		}

		graph.AddNode(&Node{
			ID:         "res:BTC/USD:forecast",
			Symbol:     "BTC/USD",
			Kind:       KindResonance,
			Value:      0.5,
			Confidence: 0.8,
			At:         at,
		})
		graph.AddNode(&Node{
			ID:         "causal:BTC/USD:uplift",
			Symbol:     "BTC/USD",
			Kind:       KindCausal,
			Value:      0.25,
			Confidence: 0.5,
			At:         at,
		})

		solver.inferStructuralEdges(types.NewThesis(nil), graph)

		Convey("It omits zero-confidence pair edges while retaining evidence-bearing edges", func() {
			counts := make(map[RelationType]int)

			for _, edge := range graph.Edges {
				counts[edge.Relation]++
			}

			So(counts[RelationStaleRelativeTo], ShouldEqual, 0)
			So(counts[RelationIncomparableWith], ShouldEqual, 0)
			So(counts[RelationSupports], ShouldEqual, 1)
			So(len(graph.Edges), ShouldEqual, 1)
		})
	})
}

func TestExtractCausalNodes(t *testing.T) {
	Convey("Given causal outputs with Pearl confidence", t, func() {
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis(nil)
		thesis.At = at
		thesis.Causal.Store("BTC/USD", map[string]any{
			"doExpectation": 0.25,
			"confidence":    0.37,
		})
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

		solver.extractCausalNodes(thesis, graph)

		Convey("It should carry confidence onto the do-expectation node", func() {
			node, found := graph.Nodes["causal:BTC/USD:doExpectation"]

			So(found, ShouldBeTrue)
			So(node.Confidence, ShouldEqual, 0.37)
		})
	})
}

func TestExtractResonanceNodes(t *testing.T) {
	Convey("Given a confidence-supported multi-step resonance forecast", t, func() {
		at := time.Unix(1, 0).UTC()
		forecast, err := types.NewResonanceForecast(
			[]float64{0.01, 0.02}, []float64{1, 0.5}, 2, 0.4,
		)
		So(err, ShouldBeNil)

		thesis := types.NewThesis(nil)
		thesis.At = at
		thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
			Symbol:   "BTC/USD",
			At:       at,
			Surprise: 0.25,
			Forecast: forecast,
		})
		graph := NewGraph(at)
		solver := NewSolver(nil, nil)

		solver.extractResonanceNodes(thesis, graph)

		Convey("It should publish the full-horizon return at measured confidence", func() {
			node, found := graph.Nodes["res:BTC/USD:forecast"]

			So(found, ShouldBeTrue)
			So(node.Value, ShouldEqual, forecast.ExpectedReturn)
			So(node.Value, ShouldNotEqual, forecast.Curve[0])
			So(node.Confidence, ShouldEqual, forecast.Confidence)
		})
	})
}

func BenchmarkInferStructuralEdges(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	nodes := make(map[string]*Node, 260)

	for index := range 256 {
		nodeID := "cat:BTC/USD:inactive:" + strconv.Itoa(index)
		nodes[nodeID] = &Node{
			ID:         nodeID,
			Symbol:     "BTC/USD",
			Kind:       KindCategory,
			Confidence: 1,
			At:         at,
		}
	}

	for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
		interventionID := "causal:" + symbol + ":intervention"
		nodes[interventionID] = &Node{
			ID:         interventionID,
			Symbol:     symbol,
			Kind:       KindCausal,
			Value:      0.5,
			Confidence: 1,
			At:         at,
		}

		expectationID := "causal:" + symbol + ":doExpectation"
		nodes[expectationID] = &Node{
			ID:         expectationID,
			Symbol:     symbol,
			Kind:       KindCausal,
			Confidence: 1,
			At:         at,
		}
	}

	solver := NewSolver(nil, nil)
	thesis := types.NewThesis(nil)

	b.ReportAllocs()

	for b.Loop() {
		graph := NewGraph(at)
		graph.Nodes = nodes
		solver.inferStructuralEdges(thesis, graph)
	}
}

func BenchmarkUpdate(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	thesis := types.NewThesis(nil)
	thesis.At = at
	forecast, err := types.NewResonanceForecast(
		[]float64{0.01},
		[]float64{1},
		1,
		0.8,
	)

	if err != nil {
		b.Fatal(err)
	}

	for _, source := range []types.SourceType{
		types.SourceCategories,
		types.SourceResonance,
		types.SourceCausal,
		types.SourceCognition,
	} {
		thesis.Readiness.Stamp(source)
	}

	for index := range 256 {
		symbol := "SIM" + strconv.Itoa(index) + "/USD"
		thesis.Categories.Store(symbol, []types.Category{{
			Symbol:     symbol,
			Type:       types.CategoryForecastEdge,
			Confidence: 0.8,
			Strength:   0.5,
			Supporting: []string{"sentiment:" + symbol + ":change"},
			Opposing:   []string{"toxicity:" + symbol + ":intensity"},
		}})
		thesis.Resonance.Store(symbol, types.ResonanceReading{
			Symbol:   symbol,
			Surprise: 0.1,
			Forecast: forecast,
		})
		thesis.Causal.Store(symbol, map[string]any{
			"association":   0.1,
			"confidence":    0.8,
			"doExpectation": 0.1,
			"intervention":  0.1,
			"uplift":        0.1,
		})
		thesis.Cognition.Store(symbol, types.Cognition{
			Symbol:     symbol,
			Source:     "cognition",
			At:         at,
			Winner:     string(types.CategoryForecastEdge),
			Confidence: 0.8,
		})
	}

	ui := make(chan []byte, 1)
	solver := NewSolver(ui, nil)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		<-ui
	}
}
