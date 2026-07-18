package types

import (
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
		thesis.Holdings.Store("BTC/USD", Holding{
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
			So(found.(Holding).Qty.Float64(), ShouldEqual, 0.25)
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
BenchmarkThesisMarshalJSON measures the checkpoint encoding used once per tick.
*/
func BenchmarkThesisMarshalJSON(b *testing.B) {
	thesis := NewThesis(nil, nil)
	thesis.Tick = 47
	thesis.Holdings.Store("BTC/USD", Holding{
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
