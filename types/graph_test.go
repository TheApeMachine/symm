package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	gonumgraph "gonum.org/v1/gonum/graph"
)

func TestGraphAddNode(t *testing.T) {
	Convey("Given a symbol-local graph", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(10, 0)
		measurement := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}

		Convey("When the same measurement is added twice", func() {
			first := graph.AddNode(measurement)
			second := graph.AddNode(measurement)

			Convey("Then only one node is retained", func() {
				So(first, ShouldBeTrue)
				So(second, ShouldBeFalse)
				So(graph.Nodes().Len(), ShouldEqual, 1)
				So(graph.At, ShouldEqual, at)
			})
		})

		Convey("When the same metric is observed at a later time", func() {
			later := *measurement
			later.At = at.Add(time.Second)

			first := graph.AddNode(measurement)
			second := graph.AddNode(&later)

			Convey("Then both evidence instances are retained", func() {
				So(first, ShouldBeTrue)
				So(second, ShouldBeTrue)
				So(graph.Nodes().Len(), ShouldEqual, 2)
			})
		})

		Convey("When pointer-backed estimator values change after insertion", func() {
			normalized := 0.5
			measurement.Normalized = &normalized
			measurement.Uncertainty = &MeasurementUncertainty{
				Lower: 0.4, Upper: 0.6,
			}
			graph.AddNode(measurement)

			normalized = 0.9
			measurement.Uncertainty.Lower = 0.8

			Convey("Then retained evidence remains immutable", func() {
				nodes := graph.Nodes()
				So(nodes.Next(), ShouldBeTrue)
				retained := nodes.Node().(*Node)
				So(*retained.Measurement.Normalized, ShouldEqual, 0.5)
				So(retained.Measurement.Uncertainty.Lower, ShouldEqual, 0.4)
			})
		})

		Convey("When a foreign symbol measurement is added", func() {
			foreign := &Measurement{
				Stream: Hawkes,
				Metric: MetricArrivalRate,
				Symbol: "ETH/USD",
				At:     at,
			}

			added := graph.AddNode(foreign)

			Convey("Then it is rejected", func() {
				So(added, ShouldBeFalse)
				So(graph.Nodes().Len(), ShouldEqual, 0)
			})
		})
	})
}

func TestGraphRelate(t *testing.T) {
	Convey("Given two measurement nodes", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(20, 0)
		from := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}
		to := &Measurement{
			Stream:  PumpDump,
			Metric:  MetricEventCount,
			Subject: SubjectTradeArrivals,
			Side:    SideNone,
			Symbol:  "BTC/USD",
			At:      at,
		}

		graph.AddNode(from)
		graph.AddNode(to)

		Convey("When they are linked with temporal context", func() {
			linked := graph.Relate(
				MeasurementKey(from),
				MeasurementKey(to),
				Supports,
				at,
				at.Add(-time.Second),
			)

			Convey("Then the edge retains provenance", func() {
				So(linked, ShouldBeTrue)
				edges := graphEdges(graph)
				So(edges, ShouldHaveLength, 1)
				So(edges[0].Type, ShouldEqual, Supports)
				So(edges[0].ObservedFrom, ShouldEqual, at.Add(-time.Second))
			})
		})

		Convey("When the same edge is related again with equivalent timestamps", func() {
			wallAt := time.Now().Truncate(time.Second)
			observedFrom := wallAt.Add(-time.Second)
			fromKey := MeasurementKey(from)
			toKey := MeasurementKey(to)

			So(graph.Relate(fromKey, toKey, Supports, wallAt, observedFrom), ShouldBeTrue)

			equivalentAt := time.Date(
				wallAt.Year(), wallAt.Month(), wallAt.Day(),
				wallAt.Hour(), wallAt.Minute(), wallAt.Second(), 0, wallAt.Location(),
			)
			equivalentObservedFrom := time.Date(
				observedFrom.Year(), observedFrom.Month(), observedFrom.Day(),
				observedFrom.Hour(), observedFrom.Minute(), observedFrom.Second(),
				0, observedFrom.Location(),
			)

			Convey("Then duplicate edges are rejected", func() {
				So(graph.Relate(fromKey, toKey, Supports, equivalentAt, equivalentObservedFrom), ShouldBeFalse)
				So(graphEdges(graph), ShouldHaveLength, 1)
			})
		})
	})
}

func graphEdges(evidenceGraph *Graph) []*Edge {
	edges := evidenceGraph.Edges()
	relations := make([]*Edge, 0)

	for edges.Next() {
		edge := edges.Edge()
		lines := evidenceGraph.Lines(edge.From().ID(), edge.To().ID())

		for lines.Next() {
			relations = append(relations, lines.Line().(*Edge))
		}
	}

	return relations
}

var _ gonumgraph.Node = (*Node)(nil)
var _ gonumgraph.Line = (*Edge)(nil)
var _ gonumgraph.DirectedMultigraph = (*Graph)(nil)

func BenchmarkGraphAddNode(b *testing.B) {
	graph := NewGraph("BTC/USD")
	measurement := &Measurement{
		Stream:  Hawkes,
		Metric:  MetricArrivalRate,
		Subject: SubjectTradeArrivals,
		Side:    SideBuy,
		Symbol:  "BTC/USD",
		At:      time.Unix(1, 0),
	}

	for b.Loop() {
		graph.AddNode(measurement)
	}
}
