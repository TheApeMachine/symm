package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func toxicityOrder(price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      "add",
		OrderID:    "order",
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func toxicityMessage(symbol string, at time.Time, bidPrice, bidQty, askPrice, askQty float64) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      []kraken.Level3Order{toxicityOrder(bidPrice, bidQty, at)},
		Asks:      []kraken.Level3Order{toxicityOrder(askPrice, askQty, at)},
	}
}

func crossedToxicityMessage(symbol string, at time.Time) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      []kraken.Level3Order{toxicityOrder(101, 10, at)},
		Asks:      []kraken.Level3Order{toxicityOrder(99, 12, at)},
	}
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a sequence of touch observations", t, func() {
		entity := NewLevel3()

		Convey("the first observation anchors the previous touch", func() {
			measurement := entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["touch_quantity:ask"].Raw, ShouldEqual, 12.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["unfilled_residual_quantity:bid"].Raw, ShouldEqual, 10.0)

			// Stateless direct measurement is whole (Maturity 1).
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes a bid retreat", func() {
			entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			measurement := entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_001, 0), 98, 5, 101, 12))

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
			entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

			measurement := entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_001, 0), 99, 4, 101, 12))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldAlmostEqual, 0.6, 1e-12)
			So(measurement.Metrics["net_replenishment_fraction:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawal_rate:bid"].Raw, ShouldAlmostEqual, 6.0, 1e-12)
			So(measurement.Metrics["withdrawal_fraction_baseline:bid"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["withdrawal_fraction_zscore:bid"].Raw, ShouldNotEqual, 0.0)
		})
	})

	Convey("Given a one-sided Level-3 update", t, func() {
		entity := NewLevel3()

		entity.Step(toxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))

		Convey("the opposite-side touch is borrowed from the last retained touch", func() {
			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: time.Unix(1_700_000_001, 0),
				Bids:      []kraken.Level3Order{toxicityOrder(98, 5, time.Unix(1_700_000_001, 0))},
			})

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 98.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["previous_best_price:ask"].Raw, ShouldEqual, 101.0)
		})
	})

	Convey("Given a crossed message", t, func() {
		entity := NewLevel3()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(crossedToxicityMessage("BTC/USD", time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})

	Convey("Given a message with no usable touch on either side", t, func() {
		entity := NewLevel3()

		Convey("Step returns a descriptive measurement error rather than panicking", func() {
			measurement := entity.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: time.Unix(1_700_000_000, 0),
			})

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
