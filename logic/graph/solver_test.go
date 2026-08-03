package graph

import (
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

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

		solver.inferStructuralEdges(types.NewThesis(), graph)

		Convey("Every intervention conditions every do-expectation", func() {
			conditionEdges := 0

			for _, edge := range graph.Edges {
				if edge.Relation == RelationConditions {
					conditionEdges++
				}
			}

			So(conditionEdges, ShouldEqual, 4)
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

		solver.inferStructuralEdges(types.NewThesis(), graph)

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
		thesis := types.NewThesis()
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

func TestPublish(t *testing.T) {
	Convey("Given a graph with related nodes", t, func() {
		at := time.Unix(1, 0).UTC()
		ui := make(chan []byte, 1)
		solver := NewSolver(ui, nil)

		graph := NewGraph(at)
		graph.AddNode(&Node{
			ID:         "res:BTC/USD:surprise",
			Symbol:     "BTC/USD",
			Kind:       KindResonance,
			Value:      0.4,
			Confidence: 1,
			At:         at,
		})
		graph.AddNode(&Node{
			ID:         "causal:BTC/USD:uplift",
			Symbol:     "BTC/USD",
			Kind:       KindCausal,
			Value:      -0.2,
			Confidence: 1,
			At:         at,
		})
		graph.AddEdge(&Edge{
			From:       "res:BTC/USD:surprise",
			To:         "causal:BTC/USD:uplift",
			Relation:   RelationContradicts,
			Weight:     0.3,
			Confidence: 1,
			At:         at,
		})

		thesis := types.NewThesis()
		solver.publish(thesis, graph)

		Convey("It should publish the edges alongside the nodes", func() {
			/*
				A graph is the relationships it encodes. Publishing the nodes
				without the edges would leave the display a list of readings
				with no way to show how any of them relate.
			*/
			var frame map[string]any

			select {
			case raw := <-ui:
				So(sonic.Unmarshal(raw, &frame), ShouldBeNil)
			default:
				t.Fatal("no graph frame published")
			}

			published, ok := frame["graph"].(map[string]any)
			So(ok, ShouldBeTrue)

			nodes, ok := published["nodes"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(len(nodes), ShouldEqual, 2)

			edges, ok := published["edges"].([]any)
			So(ok, ShouldBeTrue)
			So(len(edges), ShouldEqual, 1)

			edge, ok := edges[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(edge["from"], ShouldEqual, "res:BTC/USD:surprise")
			So(edge["to"], ShouldEqual, "causal:BTC/USD:uplift")
			So(edge["relation"], ShouldEqual, string(RelationContradicts))
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
	thesis := types.NewThesis()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		graph := NewGraph(at)
		graph.Nodes = nodes
		solver.inferStructuralEdges(thesis, graph)
	}
}

func BenchmarkPublish(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	ui := make(chan []byte, 1)
	solver := NewSolver(ui, nil)
	thesis := types.NewThesis()
	thesis.At = at
	graph := NewGraph(at)

	for index := range 256 {
		nodeID := "cat:BTC/USD:source:" + strconv.Itoa(index)
		graph.AddNode(&Node{
			ID:         nodeID,
			Symbol:     "BTC/USD",
			Source:     "source",
			Kind:       KindCategory,
			Value:      float64(index),
			Confidence: 1,
			At:         at,
			Metadata: map[string]any{
				"readiness": "observation",
				"state":     "valid",
				"unit":      "dimensionless",
			},
		})

		if index == 0 {
			continue
		}

		graph.AddEdge(&Edge{
			From:       "cat:BTC/USD:source:" + strconv.Itoa(index-1),
			To:         nodeID,
			Relation:   RelationSupports,
			Weight:     1,
			Confidence: 1,
			At:         at,
			Reason:     "benchmark relation",
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		solver.publish(thesis, graph)
		<-ui
	}
}
