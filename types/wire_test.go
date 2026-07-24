package types

import (
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestAggregateMeasurements(t *testing.T) {
	Convey("Given flat typed rows for two symbols from one signal", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricBreadth, At: at, Raw: 1,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricStrength, At: at, Raw: 0.4,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "ETH/USD",
				Metric: MetricBreadth, At: at, Raw: 0.5,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "ETH/USD",
				Metric: MetricStrength, At: at, Raw: 0.2,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
		}

		wired := AggregateMeasurements(rows)

		Convey("It emits one nested map per symbol", func() {
			So(wired, ShouldHaveLength, 2)
			So(ObservationCount(rows), ShouldEqual, 2)

			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["breadth"], ShouldEqual, 1.0)
			So(metrics["strength"], ShouldEqual, 0.4)
			So(wired[0]["source"], ShouldEqual, SourceSentiment)
			So(wired[0]["symbol"], ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given directional Hawkes metrics that share a metric name", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricArrivalRate, Side: SideBuy, At: at, Raw: 1.5,
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricArrivalRate, Side: SideSell, At: at, Raw: 0.7,
			},
		}

		wired := AggregateMeasurements(rows)

		Convey("It keeps buy and sell under distinct metric:side keys", func() {
			So(wired, ShouldHaveLength, 1)
			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["arrival_rate:buy"], ShouldEqual, 1.5)
			So(metrics["arrival_rate:sell"], ShouldEqual, 0.7)
		})
	})

	Convey("Given an older epoch for one symbol", t, func() {
		rows := []*Measurement{
			{
				Source: SourcePumpDump, Symbol: "BTC/USD",
				Metric: MetricStrength, At: time.Unix(1, 0), Raw: 0.1,
			},
			{
				Source: SourcePumpDump, Symbol: "BTC/USD",
				Metric: MetricStrength, At: time.Unix(2, 0), Raw: 0.9,
			},
		}

		wired := AggregateMeasurements(rows)

		Convey("It keeps only the newest complete epoch", func() {
			So(wired, ShouldHaveLength, 1)
			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["strength"], ShouldEqual, 0.9)
		})
	})
}

func TestFocused(t *testing.T) {
	Convey("Given measurements across two symbols", t, func() {
		rows := []*Measurement{
			{Symbol: "BTC/USD", Metric: MetricStrength, Raw: 1},
			{Symbol: "ETH/USD", Metric: MetricStrength, Raw: 2},
		}

		Convey("When focus is empty it keeps the full batch", func() {
			SetFocus("")
			So(Focused(rows), ShouldHaveLength, 2)
		})

		Convey("When focus is set it keeps only that symbol", func() {
			SetFocus("BTC/USD")
			Reset(func() { SetFocus("") })

			out := Focused(rows)
			So(out, ShouldHaveLength, 1)
			So(out[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestWireMeasurements(t *testing.T) {
	Convey("Given a UI channel and focused flat rows", t, func() {
		SetFocus("BTC/USD")
		Reset(func() { SetFocus("") })

		ui := make(chan []byte, 1)
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricBreadth, At: at, Raw: 1,
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricStrength, At: at, Raw: 0.4,
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "ETH/USD",
				Metric: MetricStrength, At: at, Raw: 0.2,
			},
		}

		WireMeasurements(rows, ui)

		Convey("It publishes one envelope with focus-aggregated metrics", func() {
			So(len(ui), ShouldEqual, 1)
			payload := <-ui

			var envelope WireEnvelope
			So(sonic.Unmarshal(payload, &envelope), ShouldBeNil)
			So(envelope.Compatible(), ShouldBeTrue)

			measurements, ok := envelope.Payload["measurements"].([]any)
			So(ok, ShouldBeTrue)
			So(measurements, ShouldHaveLength, 1)
		})
	})
}

func BenchmarkAggregateMeasurements(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
	)

	rows := make([]*Measurement, 0, symbolCount*metricCount)
	at := time.Unix(100, 0).UTC()

	for symbolIndex := range symbolCount {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for metricIndex := range metricCount {
			rows = append(rows, &Measurement{
				Source: SourceSentiment,
				Stream: Sentiment,
				Symbol: symbol,
				Metric: MetricStrength,
				At:     at,
				Raw:    float64(metricIndex),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = AggregateMeasurements(rows)
	}
}
