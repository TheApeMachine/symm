package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func spotTrade(symbol string, price float64, qty float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"price": data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(price),
		"qty":   data.NewMetric[float64]("qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(qty),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestTradeStep(t *testing.T) {
	Convey("Given a multi-leg volume-clock sequence", t, func() {
		entity := NewTrade(t.Context())
		at := time.Unix(1_700_000_000, 0)

		Convey("the opening trade seeds an open bar", func() {
			measurement := entity.Step(spotTrade("BTC/USD", 100, 2, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["trade_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["volume_bar_target_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["volume_bar_trade_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["volume_bar_duration"].Raw, ShouldEqual, 0.0)

			// An incomplete bar is not a zero-rate bar: rates are absent.
			_, hasVolumeRate := measurement.Metrics["volume_rate"]
			So(hasVolumeRate, ShouldBeFalse)
			_, hasNotionalRate := measurement.Metrics["notional_rate"]
			So(hasNotionalRate, ShouldBeFalse)

			// No previous trade exists yet, so the trade interval is absent.
			_, hasInterval := measurement.Metrics["trade_interval_seconds"]
			So(hasInterval, ShouldBeFalse)
		})

		Convey("the closing trade reports the completed bar and its throughput", func() {
			entity.Step(spotTrade("BTC/USD", 100, 2, at))
			measurement := entity.Step(spotTrade("BTC/USD", 110, 1, at.Add(5*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["volume_bar_target_quantity"].Raw, ShouldAlmostEqual, 1.5, 1e-12)
			So(measurement.Metrics["volume_bar_quantity"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["volume_bar_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["volume_bar_trade_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_duration"].Raw, ShouldEqual, 5.0)

			So(measurement.Metrics["volume_rate"].Raw, ShouldAlmostEqual, 3.0/5.0, 1e-12)
			So(measurement.Metrics["notional_rate"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-12)
			So(measurement.Metrics["trade_rate"].Raw, ShouldAlmostEqual, 2.0/5.0, 1e-12)
			So(measurement.Metrics["trade_interval_seconds"].Raw, ShouldAlmostEqual, 5.0, 1e-12)

			// The notional-rate baseline of one value is the value itself.
			So(measurement.Metrics["notional_rate_baseline"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-9)
			So(measurement.Metrics["notional_rate_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-9)
		})
	})

	Convey("Given non-positive price or quantity", t, func() {
		entity := NewTrade(t.Context())

		Convey("measurement carries the error", func() {
			measurement := entity.Step(spotTrade("BTC/USD", 0, 1, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

func TestTradeRegister(t *testing.T) {
	Convey("Given a Trade entity", t, func() {
		entity := NewTrade(t.Context())
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "pumpdump:trade")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"trade_price",
			"trade_quantity",
			"trade_notional",
			"volume_bar_target_quantity",
			"volume_bar_quantity",
			"volume_bar_notional",
			"volume_bar_trade_count",
			"volume_bar_duration",
			"completed_volume_bar_ordinal",
			"trade_interval_seconds",
			"volume_rate",
			"notional_rate",
			"trade_rate",
			"notional_rate_baseline",
			"notional_rate_ratio",
			"notional_rate_divergence",
			"notional_rate_zscore",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}
