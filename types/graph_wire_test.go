package types

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGraphFrame(t *testing.T) {
	Convey("Given a composed evidence graph", t, func() {
		graph := NewGraph("BTC/USD")
		firstAt := time.Unix(10, 0)
		secondAt := firstAt.Add(time.Second)
		first := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      firstAt,
			Raw:     0.4,
		}
		second := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideSell,
			Symbol:  "BTC/USD",
			At:      secondAt,
			Raw:     0.6,
		}

		So(graph.AddNode(first), ShouldBeNil)
		So(graph.AddNode(second), ShouldBeNil)
		So(
			graph.Relate(
				MeasurementKey(first),
				MeasurementKey(second),
				Supports,
				secondAt,
				firstAt,
			),
			ShouldBeTrue,
		)

		Convey("When serialized to a wire frame", func() {
			frame := graph.Frame()
			payload, err := json.Marshal(frame)

			Convey("Then nodes and edges remain addressable by measurement keys", func() {
				So(err, ShouldBeNil)
				So(frame.Symbol, ShouldEqual, "BTC/USD")
				So(frame.Nodes, ShouldHaveLength, 2)
				So(frame.Edges, ShouldNotBeEmpty)

				var decoded GraphFrame

				So(json.Unmarshal(payload, &decoded), ShouldBeNil)
				So(decoded.Nodes[0].Key, ShouldNotBeEmpty)
				So(decoded.Edges[0].From, ShouldNotBeEmpty)
				So(decoded.Edges[0].To, ShouldNotBeEmpty)
			})
		})

		Convey("When published through a Thesis", func() {
			uiHub := make(chan []byte, 1)
			thesis := NewThesis(uiHub)
			thesis.Graphs.Store(graph.Symbol, graph)
			thesis.Lifecycle.Store(graph.Symbol, LifecycleManaging)
			thesis.Publish()
			var payload []byte

			select {
			case payload = <-uiHub:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for Thesis publication")
			}
			published := struct {
				Graphs    []GraphFrame      `json:"graphs"`
				Lifecycle map[string]string `json:"lifecycle"`
			}{}

			Convey("Then the websocket payload contains the existing graph frame", func() {
				So(json.Unmarshal(payload, &published), ShouldBeNil)
				So(published.Graphs, ShouldHaveLength, 1)
				So(published.Graphs[0].Symbol, ShouldEqual, graph.Symbol)
				So(published.Graphs[0].Nodes, ShouldHaveLength, 2)
				So(published.Graphs[0].Edges, ShouldNotBeEmpty)
				So(published.Lifecycle[graph.Symbol], ShouldEqual, LifecycleManaging)
			})
		})
	})
}

func BenchmarkGraphFrame(b *testing.B) {
	graph := NewGraph("BTC/USD")
	at := time.Unix(10, 0)

	for index := range 32 {
		measurement := &Measurement{
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      at.Add(time.Duration(index) * time.Second),
			Raw:     float64(index) / 32,
		}

		if err := graph.AddNode(measurement); err != nil {
			b.Fatal(err)
		}
	}

	graph.Compose()

	b.ReportAllocs()

	for b.Loop() {
		_ = graph.Frame()
	}
}
