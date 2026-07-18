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

	Convey("Given a Hawkes fit epoch older than the live intensity", t, func() {
		fitFrom := time.Unix(80, 0).UTC()
		fitAt := time.Unix(100, 0).UTC()
		evalAt := time.Unix(140, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricBaselineIntensity, Side: SideBuy, At: fitAt, Raw: 0.6,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricBaselineIntensity, Side: SideSell, At: fitAt, Raw: 0.4,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricDecayRate, At: fitAt, Raw: 1.5,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricSpectralRadius, At: fitAt, Raw: 0.72,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricConditionalIntensity, Side: SideBuy, At: evalAt, Raw: 0.9,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: evalAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricConditionalIntensity, Side: SideSell, At: evalAt, Raw: 0.6,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: evalAt,
				},
			},
		}

		wired := WireMeasurements(rows)

		Convey("It publishes the fit parameters beside the live intensity", func() {
			So(wired, ShouldHaveLength, 2)

			var model, intensity datura.Map[any]

			for _, frame := range wired {
				metrics := frame["metrics"].(datura.Map[any])

				if _, ok := metrics["decay_rate"]; ok {
					model = frame
					continue
				}

				intensity = frame
			}

			So(model, ShouldNotBeNil)
			So(intensity, ShouldNotBeNil)
			So(model["metrics"].(datura.Map[any])["spectral_radius"], ShouldEqual, 0.72)
			So(intensity["metrics"].(datura.Map[any])["conditional_intensity:buy"], ShouldEqual, 0.9)

			modelScale := model["scale"].(ScaleReference)
			intensityScale := intensity["scale"].(ScaleReference)
			So(modelScale.From, ShouldEqual, fitFrom)
			So(modelScale.Through, ShouldEqual, fitAt)
			So(intensityScale.From, ShouldEqual, fitFrom)
			So(intensityScale.Through, ShouldEqual, evalAt)
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
