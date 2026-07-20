package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestThesisMarshalJSON verifies the persisted checkpoint is the Thesis itself and
that its concurrent state is usable again after restart.
*/
func TestThesisMarshalJSON(t *testing.T) {
	Convey("Given a completed Thesis tick", t, func() {
		thesis := NewThesis(nil, nil)
		thesis.Tick = 47
		thesis.Holdings.Store("BTC/USD", &Holding{
			Symbol: "BTC/USD", Asset: "BTC", Qty: decimal.NewFromFloat64(0.25),
		})
		thesis.CrossSection.Metrics = append(thesis.CrossSection.Metrics, SymbolMetric{
			Symbol: "BTC/USD", At: time.Unix(47, 0), LatestChange: 0.02,
		})
		thesis.Lifecycle.Store("BTC/USD", LifecycleEntered)
		thesis.Cognition.Store("BTC/USD", Cognition{
			Symbol: "BTC/USD", Winner: "buy", Ready: true,
		})
		graph := NewGraph("BTC/USD")
		So(graph.AddNode(&Measurement{
			Stream: Hawkes, Metric: MetricArrivalRate,
			Subject: SubjectTradeArrivals, Side: SideBuy,
			Symbol: "BTC/USD", At: time.Unix(47, 0),
		}), ShouldBeNil)
		thesis.Graphs.Store("BTC/USD", graph)

		encoded, err := sonic.Marshal(thesis)
		So(err, ShouldBeNil)

		restored := NewThesis(nil, nil)
		err = sonic.Unmarshal(encoded, restored)

		Convey("It should restore the tick and its live state containers", func() {
			found, ok := restored.Holdings.Load("BTC/USD")
			So(ok, ShouldBeTrue)

			So(err, ShouldBeNil)
			So(restored.Tick, ShouldEqual, 47)
			So(found.(*Holding).Qty.Float64(), ShouldEqual, 0.25)
			So(restored.CrossSection.Metrics, ShouldHaveLength, 1)
			So(restored.CrossSection.index["BTC/USD"], ShouldEqual, 0)
			lifecycle, lifecycleFound := restored.Lifecycle.Load("BTC/USD")
			So(lifecycleFound, ShouldBeTrue)
			So(lifecycle, ShouldEqual, LifecycleEntered)
			cognition, cognitionFound := restored.Cognition.Load("BTC/USD")
			So(cognitionFound, ShouldBeTrue)
			So(cognition.(Cognition).Winner, ShouldEqual, "buy")
			restoredGraph, graphFound := restored.Graphs.Load("BTC/USD")
			So(graphFound, ShouldBeTrue)
			So(restoredGraph.(*Graph).Nodes(), ShouldHaveLength, 1)
		})
	})
}

/*
TestThesisUnmarshalRejectsDuplicateGraphNodes proves checkpoint recovery
propagates graph identity conflicts instead of accepting ambiguous edge targets.
*/
func TestThesisUnmarshalRejectsDuplicateGraphNodes(t *testing.T) {
	Convey("Given a checkpoint with duplicate graph node keys", t, func() {
		payload, err := json.Marshal(map[string]any{
			"graphs": map[string]GraphFrame{
				"BTC/USD": {
					Symbol: "BTC/USD",
					Nodes: []GraphNodeWire{
						{Key: "duplicate", Measurement: Measurement{Symbol: "BTC/USD"}},
						{Key: "duplicate", Measurement: Measurement{Symbol: "BTC/USD"}},
					},
				},
			},
		})
		So(err, ShouldBeNil)

		var thesis Thesis
		err = sonic.Unmarshal(payload, &thesis)

		Convey("Then recovery reports the conflicting identity", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

/*
TestThesisResetCutUsesMarketTime proves virtual and live cuts share the
authoritative time passed to Market.Cut rather than reading the wall clock.
*/
func TestThesisResetCutUsesMarketTime(t *testing.T) {
	Convey("Given a market frame with an explicit cut time", t, func() {
		at := time.Unix(123, 456).UTC()
		thesis := NewThesis(nil, nil)
		thesis.ResetCut(&MarketFrame{
			At:           at,
			CrossSection: NewCrossSection(),
		}, 9)

		Convey("Then the Thesis and downstream decisions share market time", func() {
			So(thesis.At, ShouldEqual, at)
			So(thesis.Tick, ShouldEqual, int64(9))
		})
	})
}

/*
BenchmarkThesisMarshalJSON measures the checkpoint encoding used once per tick.
*/
func BenchmarkThesisMarshalJSON(b *testing.B) {
	thesis := NewThesis(nil, nil)
	thesis.Tick = 47
	thesis.Holdings.Store("BTC/USD", &Holding{
		Symbol: "BTC/USD", Asset: "BTC", Qty: decimal.NewFromFloat64(0.25),
	})
	thesis.Lifecycle.Store("BTC/USD", LifecycleEntered)
	thesis.Cognition.Store("BTC/USD", Cognition{
		Symbol: "BTC/USD", Winner: "buy", Ready: true,
	})
	b.ReportAllocs()

	for b.Loop() {
		if _, err := sonic.Marshal(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
