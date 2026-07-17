package types

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestWireMeasurements(t *testing.T) {
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

		wired := WireMeasurements(rows)

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

		wired := WireMeasurements(rows)

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

		wired := WireMeasurements(rows)

		Convey("It keeps only the newest complete epoch", func() {
			So(wired, ShouldHaveLength, 1)
			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["strength"], ShouldEqual, 0.9)
		})
	})
}

func BenchmarkWireMeasurements(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
	)

	rows := make([]*Measurement, 0, symbolCount*metricCount)
	at := time.Unix(100, 0).UTC()

	for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for metricIndex := 0; metricIndex < metricCount; metricIndex++ {
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
		if ObservationCount(rows) != symbolCount {
			b.Fatal("wire observation count drifted")
		}
	}
}
