package types

import (
	"encoding/json"
	"sync"
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
			uiHub := make(chan []byte, 4)
			thesis := NewThesis(uiHub)
			thesis.Graphs.Store(graph.Symbol, graph)
			thesis.Lifecycle.Store(graph.Symbol, LifecycleManaging)
			thesis.Publish()
			published := struct {
				Graphs    []GraphFrame      `json:"graphs"`
				Lifecycle map[string]string `json:"lifecycle"`
			}{}

			for len(uiHub) > 0 {
				frame := struct {
					Graphs    []GraphFrame      `json:"graphs"`
					Lifecycle map[string]string `json:"lifecycle"`
				}{}

				So(json.Unmarshal(<-uiHub, &frame), ShouldBeNil)

				if len(frame.Graphs) > 0 {
					published.Graphs = frame.Graphs
				}

				if len(frame.Lifecycle) > 0 {
					published.Lifecycle = frame.Lifecycle
				}
			}

			Convey("Then the websocket payload contains the existing graph frame", func() {
				So(published.Graphs, ShouldHaveLength, 1)
				So(published.Graphs[0].Symbol, ShouldEqual, graph.Symbol)
				So(published.Graphs[0].Nodes, ShouldHaveLength, 2)
				So(published.Graphs[0].Edges, ShouldNotBeEmpty)
				So(published.Lifecycle[graph.Symbol], ShouldEqual, LifecycleManaging)
			})
		})
	})
}

func TestGraphFrameConcurrentMutation(t *testing.T) {
	Convey("Given a graph published while analyzer evidence is added", t, func() {
		graph := NewGraph("BTC/USD")
		failures := make(chan error, 1)
		wait := sync.WaitGroup{}
		wait.Add(2)

		go func() {
			defer wait.Done()
			previousKey := ""

			for index := range 1_000 {
				measurement := &Measurement{
					Stream: Hawkes, Metric: MetricArrivalRate,
					Subject: SubjectTradeArrivals, Side: SideBuy,
					Symbol: "BTC/USD", At: time.Unix(int64(index), 0),
					Raw: float64(index),
				}

				if err := graph.AddNode(measurement); err != nil {
					failures <- err
					return
				}

				key := MeasurementKey(measurement)

				if previousKey != "" {
					graph.Relate(
						previousKey, key, Leads,
						measurement.At, measurement.At.Add(-time.Second),
					)
				}

				previousKey = key
			}
		}()

		go func() {
			defer wait.Done()

			for range 1_000 {
				_ = graph.Frame()
			}
		}()

		wait.Wait()

		Convey("Then publication returns a complete topology snapshot", func() {
			So(len(failures), ShouldEqual, 0)
			frame := graph.Frame()
			So(frame.Nodes, ShouldHaveLength, 1_000)
			So(frame.Edges, ShouldHaveLength, 999)
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
