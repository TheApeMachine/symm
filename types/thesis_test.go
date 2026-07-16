package types

import (
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestThesisCurrentMeasurements verifies the UI projection retains every metric
from the newest source epoch without replaying earlier observations.
*/
func TestThesisCurrentMeasurements(t *testing.T) {
	Convey("Given multiple epochs from one symbol and source", t, func() {
		thesis := NewThesis(nil)
		first := time.Unix(1, 0)
		latest := time.Unix(2, 0)
		thesis.Measurements = []*Measurement{
			{Symbol: "BTC/USD", Source: "hawkes", Metric: MetricArrivalRate, At: first},
			{Symbol: "BTC/USD", Source: "hawkes", Metric: MetricArrivalRate, At: latest},
			{Symbol: "BTC/USD", Source: "hawkes", Metric: MetricDecayRate, At: latest},
		}

		current := thesis.CurrentMeasurements()

		Convey("It should retain the complete newest epoch only", func() {
			So(current, ShouldHaveLength, 2)
			So(current[0].At, ShouldEqual, latest)
			So(current[1].At, ShouldEqual, latest)
		})
	})

	Convey("Given a focused dashboard projection", t, func() {
		thesis := NewThesis(nil)
		at := time.Unix(2, 0)
		thesis.Measurements = []*Measurement{
			{Symbol: "BTC/USD", Source: SourceFluid, Metric: MetricReynolds, At: at},
			{Symbol: "BTC/USD", Source: SourceFluid, Metric: MetricViscosity, At: at},
			{Symbol: "ETH/USD", Source: SourceFluid, Metric: MetricReynolds, At: at},
			{Symbol: "ETH/USD", Source: SourceFluid, Metric: MetricViscosity, At: at},
			{Symbol: "ETH/USD", Source: SourceHawkes, Metric: MetricStrength, At: at},
		}
		thesis.SetUIProjection("BTC/USD", SourceFluid)

		current := thesis.CurrentMeasurements()

		Convey("It should retain focused detail and one selected-source cross-section", func() {
			So(current, ShouldHaveLength, 3)
			So(current[0].Symbol, ShouldEqual, "BTC/USD")
			So(current[1].Symbol, ShouldEqual, "BTC/USD")
			So(current[2].Symbol, ShouldEqual, "ETH/USD")
			So(current[2].Metric, ShouldEqual, MetricReynolds)
		})
	})

	Convey("Given a liquidity cross-section projection", t, func() {
		thesis := NewThesis(nil)
		at := time.Unix(2, 0)
		thesis.Measurements = []*Measurement{
			{Symbol: "BTC/USD", Source: SourceLiquidity, Metric: MetricScarcityScore, At: at},
			{Symbol: "BTC/USD", Source: SourceLiquidity, Metric: MetricRelativeTouchDepth, At: at},
			{Symbol: "ETH/USD", Source: SourceLiquidity, Metric: MetricScarcityScore, At: at},
			{Symbol: "ETH/USD", Source: SourceLiquidity, Metric: MetricRelativeTouchDepth, At: at},
		}
		thesis.SetUIProjection("BTC/USD", SourceLiquidity)

		current := thesis.CurrentMeasurements()

		Convey("It should use scarcity as the peer headline", func() {
			So(current, ShouldHaveLength, 3)
			So(current[2].Symbol, ShouldEqual, "ETH/USD")
			So(current[2].Metric, ShouldEqual, MetricScarcityScore)
		})
	})
}

/*
TestThesisPublish verifies empty current-state collections remain observable so
the dashboard can distinguish an idle online system from a missing frame.
*/
func TestThesisPublish(t *testing.T) {
	Convey("Given an idle Thesis with a dashboard channel", t, func() {
		uiHub := make(chan []byte, 1)
		thesis := NewThesis(uiHub)

		thesis.Publish()

		frame := struct {
			Decisions    []Decision   `json:"decisions"`
			Orders       []spot.Order `json:"orders"`
			Positions    []Holding    `json:"positions"`
			TradeJournal []any        `json:"tradeJournal"`
			Findings     []Finding    `json:"findings"`
		}{}
		err := sonic.Unmarshal(<-uiHub, &frame)

		Convey("It should publish explicit empty current-state snapshots", func() {
			So(err, ShouldBeNil)
			So(frame.Decisions, ShouldNotBeNil)
			So(frame.Orders, ShouldNotBeNil)
			So(frame.Positions, ShouldNotBeNil)
			So(frame.TradeJournal, ShouldNotBeNil)
			So(frame.Findings, ShouldNotBeNil)
		})
	})
}

/*
TestThesisMarshalJSON verifies the persisted checkpoint is the Thesis itself and
that its concurrent state is usable again after restart.
*/
func TestThesisMarshalJSON(t *testing.T) {
	Convey("Given a completed Thesis tick", t, func() {
		thesis := NewThesis(nil)
		thesis.Tick = 47
		thesis.SetUIProjection("BTC/USD", SourceFluid)
		thesis.Positions = append(thesis.Positions, Holding{
			Symbol: "BTC/USD", Asset: "BTC", Qty: decimal.NewFromFloat64(0.25),
		})
		thesis.CrossSection.Metrics = append(thesis.CrossSection.Metrics, SymbolMetric{
			Symbol: "BTC/USD", At: time.Unix(47, 0), LatestChange: 0.02,
		})
		thesis.Signals.Store("BTC/USD", map[string]any{"strength": 0.75})
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

		restored := NewThesis(nil)
		err = sonic.Unmarshal(encoded, restored)

		Convey("It should restore the tick and its live state containers", func() {
			So(err, ShouldBeNil)
			So(restored.Tick, ShouldEqual, 47)
			So(restored.Positions, ShouldHaveLength, 1)
			So(restored.Positions[0].Qty.Float64(), ShouldEqual, 0.25)
			So(restored.CrossSection.Metrics, ShouldHaveLength, 1)
			So(restored.CrossSection.index["BTC/USD"], ShouldEqual, 0)
			_, signalFound := restored.Signals.Load("BTC/USD")
			So(signalFound, ShouldBeTrue)
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
	thesis := NewThesis(nil)
	thesis.Tick = 47
	thesis.Positions = append(thesis.Positions, Holding{
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

func BenchmarkThesisCurrentMeasurements(b *testing.B) {
	thesis := NewThesis(nil)
	at := time.Unix(1, 0)
	sources := []SourceType{
		SourceCorrelation,
		SourceCVD,
		SourceDepthFlow,
		SourceExhaustion,
		SourceFluid,
		SourceHawkes,
		SourceLeadLag,
		SourceLiquidity,
		SourcePumpDump,
		SourceSentiment,
		SourceToxicity,
	}

	for symbolIndex := range 642 {
		symbol := "SYMBOL-" + strconv.Itoa(symbolIndex) + "/USD"

		for _, source := range sources {
			for metricIndex := range 8 {
				metric := MetricValue

				if metricIndex == 0 {
					metric = MetricStrength
				}

				if source == SourceFluid && metricIndex == 0 {
					metric = MetricReynolds
				}

				if source == SourceLiquidity && metricIndex == 0 {
					metric = MetricScarcityScore
				}

				thesis.Measurements = append(thesis.Measurements, &Measurement{
					Source: source,
					Metric: metric,
					Symbol: symbol,
					At:     at,
				})
			}
		}
	}

	thesis.SetUIProjection("SYMBOL-0/USD", SourceFluid)
	b.ReportAllocs()

	for b.Loop() {
		_ = thesis.CurrentMeasurements()
	}
}
