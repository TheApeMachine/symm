package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func tradeRow(
	symbol, side string,
	price, qty float64,
	bidPrice, askPrice, bidQty, askQty float64,
	at time.Time,
) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"price":               data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(price),
		"qty":                 data.NewMetric[float64]("qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(qty),
		"best_price:bid":      data.NewMetric[float64]("best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bidPrice),
		"best_price:ask":      data.NewMetric[float64]("best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(askPrice),
		"touch_quantity:bid":  data.NewMetric[float64]("touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(bidQty),
		"touch_quantity:ask":  data.NewMetric[float64]("touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(askQty),
	})
	m.Label, m.At, m.From = symbol, at, at
	m.Provenance = map[string]string{"side": side}

	return m
}

func TestTradeStep(t *testing.T) {
	Convey("Given a touch of 100/102", t, func() {
		entity := NewTrade(t.Context())
		const bidPrice, askPrice, bidQty, askQty = 100.0, 102.0, 10.0, 20.0

		Convey("a sell at the bid touch attributes a fill", func() {
			measurement := entity.Step(tradeRow("BTC/USD", "sell", 100, 3, bidPrice, askPrice, bidQty, askQty, time.Unix(1_700_000_001, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["bracket_trade_quantity"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["matched_touch_trade_quantity:bid"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["matched_touch_trade_quantity:ask"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["touch_fill_quantity:bid"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["touch_fill_fraction:bid"].Raw, ShouldAlmostEqual, 0.3, 1e-12)

			// The first trade has no spacing, so its fill rate is undefined.
			_, hasRate := measurement.Metrics["touch_fill_rate:bid"]
			So(hasRate, ShouldBeFalse)
		})

		Convey("a later matching trade accumulates the bracket and rate", func() {
			entity.Step(tradeRow("BTC/USD", "sell", 100, 3, bidPrice, askPrice, bidQty, askQty, time.Unix(1_700_000_001, 0)))

			measurement := entity.Step(tradeRow("BTC/USD", "sell", 100, 2, bidPrice, askPrice, bidQty, askQty, time.Unix(1_700_000_002, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["bracket_trade_quantity"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["matched_touch_trade_quantity:bid"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["touch_fill_quantity:bid"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["touch_fill_fraction:bid"].Raw, ShouldAlmostEqual, 0.5, 1e-12)
			So(measurement.Metrics["touch_fill_rate:bid"].Raw, ShouldAlmostEqual, 5.0, 1e-12)
			So(measurement.Metrics["fill_fraction_baseline:bid"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["fill_fraction_divergence:bid"].Raw, ShouldNotEqual, 0.0)
		})

		Convey("the first trade reports no SNR, its estimator having no baseline yet", func() {
			measurement := entity.Step(tradeRow("BTC/USD", "sell", 100, 3, bidPrice, askPrice, bidQty, askQty, time.Unix(1_700_000_001, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.SNRDefined, ShouldBeFalse)
		})

		Convey("a varying fill fraction yields a defined SNR once the estimator settles", func() {
			var measurement *data.Measurement[float64]

			for step := range 12 {
				at := time.Unix(1_700_000_001+int64(step), 0)
				quantity := 2.0 + float64(step%3)

				measurement = entity.Step(tradeRow("BTC/USD", "sell", 100, quantity, bidPrice, askPrice, bidQty, askQty, at))
			}

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("a buy away from the ask touch does not match", func() {
			measurement := entity.Step(tradeRow("BTC/USD", "buy", 101, 4, bidPrice, askPrice, bidQty, askQty, time.Unix(1_700_000_001, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["matched_touch_trade_quantity:ask"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["touch_fill_quantity:ask"].Raw, ShouldEqual, 0.0)
		})
	})
}

func TestTradeRegister(t *testing.T) {
	Convey("Given a Trade entity", t, func() {
		entity := NewTrade(t.Context())
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "toxicity:trade")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"bracket_trade_quantity",
			"matched_touch_trade_quantity:bid",
			"matched_touch_trade_quantity:ask",
			"touch_fill_quantity:bid",
			"touch_fill_quantity:ask",
			"touch_fill_fraction:bid",
			"touch_fill_fraction:ask",
			"touch_fill_rate:bid",
			"touch_fill_rate:ask",
			"fill_fraction_baseline:bid",
			"fill_fraction_divergence:bid",
			"fill_fraction_zscore:bid",
			"fill_fraction_baseline:ask",
			"fill_fraction_divergence:ask",
			"fill_fraction_zscore:ask",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}
