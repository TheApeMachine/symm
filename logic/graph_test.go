package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGraphAddNode(t *testing.T) {
	Convey("Given a symbol-local graph", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(10, 0)
		measurement := &types.Measurement{
			Stream:  types.Hawkes,
			Metric:  types.MetricArrivalRate,
			Subject: types.SubjectTradeArrivals,
			Side:    types.SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}

		Convey("When the same measurement is added twice", func() {
			first := graph.AddNode(measurement)
			second := graph.AddNode(measurement)

			Convey("Then only one node is retained", func() {
				So(first, ShouldBeTrue)
				So(second, ShouldBeFalse)
				So(graph.Nodes, ShouldHaveLength, 1)
				So(graph.At, ShouldEqual, at)
			})
		})

		Convey("When a foreign symbol measurement is added", func() {
			foreign := &types.Measurement{
				Stream: types.Hawkes,
				Metric: types.MetricArrivalRate,
				Symbol: "ETH/USD",
				At:     at,
			}

			added := graph.AddNode(foreign)

			Convey("Then it is rejected", func() {
				So(added, ShouldBeFalse)
				So(graph.Nodes, ShouldBeEmpty)
			})
		})
	})
}

func TestGraphRelate(t *testing.T) {
	Convey("Given two measurement nodes", t, func() {
		graph := NewGraph("BTC/USD")
		at := time.Unix(20, 0)
		from := &types.Measurement{
			Stream:  types.Hawkes,
			Metric:  types.MetricArrivalRate,
			Subject: types.SubjectTradeArrivals,
			Side:    types.SideBuy,
			Symbol:  "BTC/USD",
			At:      at,
		}
		to := &types.Measurement{
			Stream:  types.PumpDump,
			Metric:  types.MetricEventCount,
			Subject: types.SubjectTradeArrivals,
			Side:    types.SideNone,
			Symbol:  "BTC/USD",
			At:      at,
		}

		graph.AddNode(from)
		graph.AddNode(to)

		Convey("When they are linked with temporal context", func() {
			linked := graph.Relate(
				measurementKey(from),
				measurementKey(to),
				Supports,
				at,
				at.Add(-time.Second),
			)

			Convey("Then the edge retains provenance", func() {
				So(linked, ShouldBeTrue)
				So(graph.Edges, ShouldHaveLength, 1)
				So(graph.Edges[0].Type, ShouldEqual, Supports)
				So(graph.Edges[0].ObservedFrom, ShouldEqual, at.Add(-time.Second))
			})
		})
	})
}

func BenchmarkGraphAddNode(b *testing.B) {
	graph := NewGraph("BTC/USD")
	measurement := &types.Measurement{
		Stream:  types.Hawkes,
		Metric:  types.MetricArrivalRate,
		Subject: types.SubjectTradeArrivals,
		Side:    types.SideBuy,
		Symbol:  "BTC/USD",
		At:      time.Unix(1, 0),
	}

	for b.Loop() {
		graph.AddNode(measurement)
	}
}
