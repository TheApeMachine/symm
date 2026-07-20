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
			Source:  SourceHawkes,
			Stream:  Hawkes,
			Metric:  MetricArrivalRate,
			Subject: SubjectTradeArrivals,
			Side:    SideBuy,
			Symbol:  "BTC/USD",
			At:      firstAt,
			Raw:     0.4,
		}
		second := &Measurement{
			Source:  SourceFluid,
			Stream:  Fluid,
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
	})
}

/*
TestEvidenceComposerRestoreNode proves malformed and conflicting wire identities are
rejected instead of silently changing the reconstructed graph topology.
*/
func TestEvidenceComposerRestoreNode(t *testing.T) {
	Convey("Given a graph reconstructed from wire nodes", t, func() {
		graph := NewGraph("BTC/USD")
		measurement := Measurement{Symbol: "BTC/USD"}

		Convey("It rejects an empty key", func() {
			So(graph.Evidence.RestoreNode("", NodeMeasurement, "", measurement), ShouldNotBeNil)
		})

		Convey("It rejects a duplicate key", func() {
			So(graph.Evidence.RestoreNode("node", NodeMeasurement, "", measurement), ShouldBeNil)
			So(graph.Evidence.RestoreNode("node", NodeMeasurement, "", measurement), ShouldNotBeNil)
		})
	})
}

func BenchmarkGraphFrame(b *testing.B) {
	graph := NewGraph("BTC/USD")
	at := time.Unix(10, 0)

	for index := range 32 {
		normalized := float64(index+1) / 32
		measurement := &Measurement{
			Source:     SourceHawkes,
			Stream:     Hawkes,
			Metric:     MetricArrivalRate,
			Subject:    SubjectTradeArrivals,
			Side:       SideBuy,
			Symbol:     "BTC/USD",
			At:         at.Add(time.Duration(index) * time.Second),
			Raw:        float64(index) / 32,
			Normalized: &normalized,
			Validity:   MeasurementValidity{State: ValidityValid},
			Unit:       UnitEventsPerSecond,
			Scale:      ScaleReference{Kind: ScaleObservationWindow},
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
