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
	Convey("Given merged rows for two symbols from one signal", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Symbol: "BTC/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBreadth, SideNone):  {Raw: 1},
					MetricKey(MetricStrength, SideNone): {Raw: 0.4},
				},
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Symbol: "ETH/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBreadth, SideNone):  {Raw: 0.5},
					MetricKey(MetricStrength, SideNone): {Raw: 0.2},
				},
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
		rows := []*Measurement{{
			Source: SourceHawkes, Symbol: "BTC/USD", At: at,
			Metrics: map[string]MetricSample{
				MetricKey(MetricArrivalRate, SideBuy):  {Raw: 1.5},
				MetricKey(MetricArrivalRate, SideSell): {Raw: 0.7},
			},
		}}

		wired := AggregateMeasurements(rows)

		Convey("It keeps buy and sell under distinct metric:side keys", func() {
			So(wired, ShouldHaveLength, 1)
			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["arrival_rate:buy"], ShouldEqual, 1.5)
			So(metrics["arrival_rate:sell"], ShouldEqual, 0.7)
		})
	})

	Convey("Given toxicity touch quantities with per-side normalization", t, func() {
		at := time.Unix(100, 0).UTC()
		bidNorm := 0.6
		askNorm := 0.4
		rows := []*Measurement{{
			Source: SourceToxicity, Symbol: "BTC/USD", At: at,
			Metrics: map[string]MetricSample{
				MetricKey(MetricTouchQuantity, SideBuy): {
					Raw: 1.5, Normalized: &bidNorm,
				},
				MetricKey(MetricTouchQuantity, SideSell): {
					Raw: 1.0, Normalized: &askNorm,
				},
			},
		}}

		wired := AggregateMeasurements(rows)

		Convey("It retains normalized samples beside raw metric:side keys", func() {
			So(wired, ShouldHaveLength, 1)
			metrics := wired[0]["metrics"].(datura.Map[any])
			So(metrics["touch_quantity:buy"], ShouldEqual, 1.5)
			So(metrics["touch_quantity:sell"], ShouldEqual, 1.0)

			normalized, ok := wired[0]["normalized_metrics"].(datura.Map[any])
			So(ok, ShouldBeTrue)
			So(normalized["touch_quantity:buy"], ShouldEqual, 0.6)
			So(normalized["touch_quantity:sell"], ShouldEqual, 0.4)
		})
	})

	Convey("Given an older epoch for one symbol", t, func() {
		rows := []*Measurement{
			{
				Source: SourcePumpDump, Symbol: "BTC/USD", At: time.Unix(1, 0),
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 0.1},
				},
			},
			{
				Source: SourcePumpDump, Symbol: "BTC/USD", At: time.Unix(2, 0),
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 0.9},
				},
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
			{
				Symbol: "BTC/USD",
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 1},
				},
			},
			{
				Symbol: "ETH/USD",
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 2},
				},
			},
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
	Convey("Given a UI channel and focused merged rows", t, func() {
		SetFocus("BTC/USD")
		Reset(func() { SetFocus("") })

		ui := make(chan []byte, 1)
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Symbol: "BTC/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBreadth, SideNone):  {Raw: 1},
					MetricKey(MetricStrength, SideNone): {Raw: 0.4},
				},
			},
			{
				Source: SourceSentiment, Symbol: "ETH/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 0.2},
				},
			},
		}

		WireMeasurements(rows, ui)

		Convey("It publishes one flat measurements frame for the focus", func() {
			So(len(ui), ShouldEqual, 1)
			payload := <-ui

			var frame map[string]any
			So(sonic.Unmarshal(payload, &frame), ShouldBeNil)

			measurements, ok := frame["measurements"].([]any)
			So(ok, ShouldBeTrue)
			So(measurements, ShouldHaveLength, 1)
			So(frame["v"], ShouldBeNil)
			So(frame["payload"], ShouldBeNil)
		})
	})
}

func BenchmarkAggregateMeasurements(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
	)

	rows := make([]*Measurement, 0, symbolCount)
	at := time.Unix(100, 0).UTC()

	for symbolIndex := range symbolCount {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)
		metrics := make(map[string]MetricSample, metricCount)

		for metricIndex := range metricCount {
			metrics[MetricKey(MetricStrength, SideNone)] = MetricSample{
				Raw: float64(metricIndex),
			}
		}

		rows = append(rows, &Measurement{
			Source:  SourceSentiment,
			Symbol:  symbol,
			At:      at,
			Metrics: metrics,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = AggregateMeasurements(rows)
	}
}
