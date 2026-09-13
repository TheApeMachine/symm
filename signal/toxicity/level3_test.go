package toxicity

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func toxicityTouch(symbol string, at time.Time, bidPrice, bidQty, askPrice, askQty float64) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"best_price:bid":     data.NewMetric[float64]("best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bidPrice),
		"best_price:ask":     data.NewMetric[float64]("best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(askPrice),
		"touch_quantity:bid": data.NewMetric[float64]("touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(bidQty),
		"touch_quantity:ask": data.NewMetric[float64]("touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(askQty),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a sequence of touch observations", t, func() {
		entity := NewLevel3(t.Context())

		Convey("the first observation anchors the previous touch", func() {
			measurement := entity.Step(toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["touch_quantity:ask"].Raw, ShouldEqual, 12.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["unfilled_residual_quantity:bid"].Raw, ShouldEqual, 10.0)

			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes a bid retreat", func() {
			first := time.Unix(1_700_000_000, 0)
			second := time.Unix(1_700_000_001, 0)

			entity.Step(toxicityTouch("BTC/USD", first, 99, 10, 101, 12))
			measurement := entity.Step(toxicityTouch("BTC/USD", second, 98, 5, 101, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["previous_best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 98.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldAlmostEqual, math.Log(98.0/99.0), 1e-12)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_fraction:bid"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_rate:bid"].Raw, ShouldAlmostEqual, 10.0, 1e-12)
		})

		Convey("a later observation attributes an unchanged-touch withdrawal", func() {
			entity.Step(toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))
			measurement := entity.Step(toxicityTouch("BTC/USD", time.Unix(1_700_000_001, 0), 99, 4, 101, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldAlmostEqual, 0.6, 1e-12)
		})
	})

	Convey("Given a crossed touch", t, func() {
		entity := NewLevel3(t.Context())

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 101, 10, 99, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		entity := NewLevel3(t.Context())
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "toxicity:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"best_price:bid",
			"best_price:ask",
			"touch_quantity:bid",
			"touch_quantity:ask",
			"unfilled_residual_quantity:bid",
			"unfilled_residual_quantity:ask",
			"previous_best_price:bid",
			"previous_best_price:ask",
			"touch_price_log_change:bid",
			"touch_price_log_change:ask",
			"retreated_quantity:bid",
			"net_withdrawn_quantity:bid",
			"net_replenished_quantity:bid",
			"retreat_fraction:bid",
			"net_withdrawal_fraction:bid",
			"retreat_rate:bid",
			"retreated_quantity:ask",
			"net_withdrawn_quantity:ask",
			"net_replenished_quantity:ask",
			"retreat_fraction:ask",
			"net_withdrawal_fraction:ask",
			"retreat_rate:ask",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}
