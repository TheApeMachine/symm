package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
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
			firstErr := graph.AddNode(measurement)
			secondErr := graph.AddNode(measurement)

			Convey("Then only one node is retained", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(graph.Nodes(), ShouldHaveLength, 1)
				So(graph.At, ShouldEqual, at)
			})
		})

		Convey("When the same metric is observed at a later time", func() {
			later := *measurement
			later.At = at.Add(time.Second)

			firstErr := graph.AddNode(measurement)
			secondErr := graph.AddNode(&later)

			Convey("Then both evidence instances are retained", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(graph.Nodes(), ShouldHaveLength, 2)
			})
		})

		Convey("When pointer-backed estimator values change after insertion", func() {
			normalized := 0.5
			measurement.Normalized = &normalized
			measurement.Uncertainty = &MeasurementUncertainty{
				Lower: 0.4, Upper: 0.6,
			}
			So(graph.AddNode(measurement), ShouldBeNil)

			normalized = 0.9
			measurement.Uncertainty.Lower = 0.8

			Convey("Then retained evidence remains immutable", func() {
				retained := graph.Nodes()[0]
				So(*retained.Measurement.Normalized, ShouldEqual, 0.5)
				So(retained.Measurement.Uncertainty.Lower, ShouldEqual, 0.4)
			})
		})

		Convey("When a nil measurement is added", func() {
			err := graph.AddNode(nil)

			Convey("Then validation rejects it before insertion", func() {
				So(err, ShouldNotBeNil)
				So(errnie.IsValidation(err), ShouldBeTrue)
				So(graph.Nodes(), ShouldBeEmpty)
			})
		})

		Convey("When a foreign symbol measurement is added", func() {
			foreign := &Measurement{
				Stream:  Hawkes,
				Metric:  MetricArrivalRate,
				Subject: SubjectTradeArrivals,
				Symbol:  "ETH/USD",
				At:      at,
			}

			err := graph.AddNode(foreign)

			Convey("Then it is rejected", func() {
				So(err, ShouldNotBeNil)
				So(errnie.IsValidation(err), ShouldBeTrue)
				So(graph.Nodes(), ShouldBeEmpty)
			})
		})

		Convey("When a normalized value is non-finite", func() {
			nonFinite := math.NaN()
			measurement.Normalized = &nonFinite

			err := graph.AddNode(measurement)

			Convey("Then structural validation rejects it before insertion", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "Normalized: must be finite")
				So(graph.Nodes(), ShouldBeEmpty)
			})
		})

		Convey("When the evidence interval runs backwards", func() {
			measurement.ObservedFrom = at.Add(time.Second)

			err := graph.AddNode(measurement)

			Convey("Then provenance is rejected at the boundary", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "observedFrom after At")
				So(graph.Nodes(), ShouldBeEmpty)
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

		So(graph.AddNode(from), ShouldBeNil)
		So(graph.AddNode(to), ShouldBeNil)

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

/*
TestGraphRelationshipTypes verifies the domain graph accepts every relationship
defined by the evidence contract without losing its references or time span.
*/
func TestGraphRelationshipTypes(t *testing.T) {
	Convey("Given two referenced evidence nodes", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(20, 0)
		from := &Measurement{Symbol: "BTC/USD", At: at, Source: SourceHawkes}
		to := &Measurement{Symbol: "BTC/USD", At: at, Source: SourceFluid}
		So(graph.AddNode(from), ShouldBeNil)
		So(graph.AddNode(to), ShouldBeNil)
		fromKey := MeasurementKey(from)
		toKey := MeasurementKey(to)
		relationships := []EdgeType{
			Supports,
			Contradicts,
			Conditions,
			Leads,
			Lags,
			Redundant,
			Independent,
			Stale,
			Incomparable,
		}

		for _, relationship := range relationships {
			So(
				graph.Relate(fromKey, toKey, relationship, at, at.Add(-time.Second)),
				ShouldBeTrue,
			)
		}

		Convey("Every edge should retain evidence references and temporal context", func() {
			So(graph.Edges(), ShouldHaveLength, len(relationships))

			for index, edge := range graph.Edges() {
				So(edge.From, ShouldEqual, fromKey)
				So(edge.To, ShouldEqual, toKey)
				So(edge.Type, ShouldEqual, relationships[index])
				So(edge.At, ShouldEqual, at)
				So(edge.ObservedFrom, ShouldEqual, at.Add(-time.Second))
			}
		})
	})
}

func graphEdges(evidenceGraph *Graph) []*Edge {
	return evidenceGraph.Edges()
}

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
		if err := graph.AddNode(measurement); err != nil {
			b.Fatal(err)
		}
	}
}
