package graph

import (
	"strconv"
	"testing"
	"time"

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

	Convey("Given stale and zero-confidence measurement nodes", t, func() {
		at := time.Unix(10, 0).UTC()
		graph := NewGraph(at)
		solver := NewSolver(nil, nil, WithStaleThreshold(time.Second))

		for index := range 256 {
			nodeID := "meas:BTC/USD:invalid:" + strconv.Itoa(index)
			graph.AddNode(&Node{
				ID:     nodeID,
				Symbol: "BTC/USD",
				Kind:   KindMeasurement,
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

func TestExtractMeasurementNodes(t *testing.T) {
	Convey("Given repeated measurements for the same metric node", t, func() {
		thesis := types.NewThesis()
		graph := NewGraph(time.Unix(3, 0).UTC())
		solver := NewSolver(nil, nil)
		rows := make([]*types.Measurement, 0, 3)

		for index := range 3 {
			rows = append(rows, &types.Measurement{
				Source: types.SourceCVD,
				Symbol: "BTC/USD",
				At:     time.Unix(int64(index+1), 0).UTC(),
				Metrics: map[string]types.MetricSample{
					"flow": {Raw: float64(index + 1), Unit: types.UnitBaseCurrency},
				},
			})
		}

		thesis.Measurements.Store(types.SourceCVD, rows)
		solver.extractMeasurementNodes(thesis, graph)

		Convey("It materializes only the final value represented by the graph ID", func() {
			node, found := graph.Nodes["meas:BTC/USD:cvd:flow"]

			So(found, ShouldBeTrue)
			So(len(graph.Nodes), ShouldEqual, 1)
			So(node.Value, ShouldEqual, 3)
			So(node.At, ShouldResemble, time.Unix(3, 0).UTC())
		})
	})
}

func BenchmarkInferStructuralEdges(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	nodes := make(map[string]*Node, 260)

	for index := range 256 {
		nodeID := "meas:BTC/USD:source:" + strconv.Itoa(index)
		nodes[nodeID] = &Node{
			ID:         nodeID,
			Symbol:     "BTC/USD",
			Kind:       KindMeasurement,
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

func BenchmarkExtractMeasurementNodes(b *testing.B) {
	at := time.Unix(1, 0).UTC()
	rows := make([]*types.Measurement, 4096)

	for index := range rows {
		rows[index] = &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			At:     at.Add(time.Duration(index) * time.Millisecond),
			Metrics: map[string]types.MetricSample{
				"flow": {Raw: float64(index), Unit: types.UnitBaseCurrency},
			},
		}
	}

	thesis := types.NewThesis()
	thesis.Measurements.Store(types.SourceCVD, rows)
	solver := NewSolver(nil, nil)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		solver.extractMeasurementNodes(thesis, NewGraph(at))
	}
}
