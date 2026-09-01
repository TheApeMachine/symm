package derivatives

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func futuresTrade(
	symbol string,
	price float64,
	qty float64,
	side string,
	tradeType string,
	at time.Time,
) kraken.FuturesTradeData {
	return kraken.FuturesTradeData{
		Symbol:    symbol,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Side:      side,
		Type:      tradeType,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a multi-leg liquidation sequence", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		Convey("a single buy liquidation accounts its interval", func() {
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "liquidation", at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["liquidation_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_notional:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_signed_fraction"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldAlmostEqual, 1.0, 1e-12)

			// The first trade opens the interval: no positive duration yet.
			_, hasRate := measurement.Metrics["liquidation_notional_rate"]
			So(hasRate, ShouldBeFalse)

			// One retained trade is still immature support.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a follow-up sell liquidation extends the interval", func() {
			entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "liquidation", at))
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 110, 1, "sell", "liquidation", at.Add(5*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["liquidation_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_notional:sell"].Raw, ShouldEqual, 110.0)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 90.0)
			So(measurement.Metrics["liquidation_signed_fraction"].Raw, ShouldAlmostEqual, 90.0/310.0, 1e-12)
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-12)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
		})
	})

	Convey("Given a non-liquidation trade", t, func() {
		entity := NewTrade()

		Convey("gross liquidation is a valid zero and the signed fraction is omitted", func() {
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "trade", time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 0.0)
			_, hasFraction := measurement.Metrics["liquidation_signed_fraction"]
			So(hasFraction, ShouldBeFalse)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 200.0)
		})
	})
}

func TestTradeStep_LateTrade(t *testing.T) {
	Convey("Given a real trade timestamp older than the last seen", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "liquidation", at)).Err, ShouldBeNil)

		opened := entity.Step(futuresTrade(
			"PF_XBTUSD", 100, 1, "sell", "liquidation", at.Add(10*time.Second),
		))
		So(opened.Err, ShouldBeNil)

		rateBefore := opened.Metrics["liquidation_notional_rate"].Raw

		Convey("the late trade is accounted without advancing the event clock", func() {
			// A real timestamp from five seconds INSIDE the open interval.
			measurement := entity.Step(futuresTrade(
				"PF_XBTUSD", 100, 3, "buy", "liquidation", at.Add(5*time.Second),
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			// The notional is a sum: order-independent, so it must be counted.
			So(measurement.Metrics["liquidation_notional:buy"].Raw, ShouldEqual, 500.0)
			So(measurement.Metrics["liquidation_notional:sell"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 600.0)

			// The rate is ABSENT, not stale and not fabricated. It divides by
			// an interval duration this event may not advance, so there is no
			// honest value to publish: re-stamping the trade forward would have
			// shortened the interval and inflated the rate, while leaving the
			// slot untouched would have republished the previous frame's number
			// under this event's identity.
			_, hasRate := measurement.Metrics["liquidation_notional_rate"]
			So(hasRate, ShouldBeFalse)
			So(rateBefore, ShouldAlmostEqual, 30.0, 1e-9)

			// The event-clock derivative has no valid difference to report for
			// an out-of-order observation, so it is absent rather than wrong.
			_, hasVelocity := measurement.Metrics["liquidation_share_velocity"]
			So(hasVelocity, ShouldBeFalse)
		})

		Convey("a later in-order trade still advances the clock normally", func() {
			entity.Step(futuresTrade("PF_XBTUSD", 100, 3, "buy", "liquidation", at.Add(5*time.Second)))

			measurement := entity.Step(futuresTrade(
				"PF_XBTUSD", 100, 1, "sell", "liquidation", at.Add(20*time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			// The interval now runs the full 20s from the origin.
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 35.0, 1e-9)
		})
	})
}

func TestTradeStep_SyntheticTimestamp(t *testing.T) {
	Convey("Given a payload that carried no server timestamp", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTrade(
			"PF_XBTUSD", 100, 2, "buy", "liquidation", at.Add(time.Hour),
		)).Err, ShouldBeNil)

		Convey("its fabricated clock is folded forward instead of read as late", func() {
			// The wall-clock substitute reads as older than the exchange time,
			// but it holds no truth, so it is pinned to the timeline head and
			// the event counts as the newest observation.
			late := futuresTrade("PF_XBTUSD", 100, 1, "sell", "liquidation", at)
			late.SyntheticTimestamp = true

			measurement := entity.Step(late)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 300.0)
		})
	})
}

func TestTradeStep_PerSymbolTimeline(t *testing.T) {
	Convey("Given trades on two symbols", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(futuresTrade(
			"PF_XBTUSD", 100, 2, "buy", "liquidation", at.Add(time.Hour),
		)).Err, ShouldBeNil)

		Convey("each symbol keeps its own timeline", func() {
			So(entity.Step(futuresTrade("PF_RAREUSD", 100, 2, "buy", "liquidation", at)).Err, ShouldBeNil)

			measurement := entity.Step(futuresTrade(
				"PF_RAREUSD", 100, 1, "sell", "liquidation", at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			// The second symbol advanced normally: its rate is defined.
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 300.0, 1e-9)
		})
	})
}
