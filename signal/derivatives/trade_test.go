package derivatives

import (
	"maps"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
tradeSchema is the register's declared metric set the workload's data
management hands the signal: every producible metric, none valued.
*/
var tradeSchema = new(Trade).Register().Metrics

/*
tradeRow builds the measurement a futures trade row lifts into: the feed
fills the price and quantity metrics, carries the categorical side and trade
type in provenance, and names the symbol and venue timestamp.
*/
func tradeRow(symbol string, price, qty float64, side, tradeType string, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"price": data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(price),
		"qty":   data.NewMetric[float64]("qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(qty),
	})
	m.Label, m.At, m.From = symbol, at, at
	m.Provenance = map[string]string{"side": side, "type": tradeType}

	return m
}

/*
syntheticTradeRow marks the measurement's timestamp as a local wall-clock
substitute for a payload that carried no server time.
*/
func syntheticTradeRow(symbol string, price, qty float64, side, tradeType string, at time.Time) *data.Measurement[float64] {
	m := tradeRow(symbol, price, qty, side, tradeType, at)
	m.Provenance["synthetic_timestamp"] = "true"

	return m
}

func TestTradeStep(t *testing.T) {
	Convey("Given a multi-leg liquidation sequence", t, func() {
		entity := NewTrade(t.Context())
		at := time.Unix(1_700_000_000, 0)

		Convey("a single buy liquidation accounts its interval", func() {
			measurement := entity.Step(tradeRow("PF_XBTUSD", 100, 2, "buy", "liquidation", at))

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
			entity.Step(tradeRow("PF_XBTUSD", 100, 2, "buy", "liquidation", at))
			measurement := entity.Step(tradeRow("PF_XBTUSD", 110, 1, "sell", "liquidation", at.Add(5*time.Second)))

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
		entity := NewTrade(t.Context())

		Convey("gross liquidation is a valid zero and the signed fraction is omitted", func() {
			measurement := entity.Step(tradeRow("PF_XBTUSD", 100, 2, "buy", "trade", time.Unix(1_700_000_000, 0)))

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
		entity := NewTrade(t.Context())
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(tradeRow("PF_XBTUSD", 100, 2, "buy", "liquidation", at)).Err, ShouldBeNil)

		opened := entity.Step(tradeRow("PF_XBTUSD", 100, 1, "sell", "liquidation", at.Add(10*time.Second)))
		So(opened.Err, ShouldBeNil)

		rateBefore := opened.Metrics["liquidation_notional_rate"].Raw

		Convey("the late trade is accounted without advancing the event clock", func() {
			// A real timestamp from five seconds INSIDE the open interval.
			measurement := entity.Step(tradeRow("PF_XBTUSD", 100, 3, "buy", "liquidation", at.Add(5*time.Second)))

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

		Convey("a reconnect trade predating the interval remains valid through Category", func() {
			historicalAt := at.Add(-30 * time.Second)
			measurement := entity.Step(tradeRow("PF_XBTUSD", 100, 3, "buy", "liquidation", historicalAt))
			So(measurement.Err, ShouldBeNil)
			So(measurement.From.Equal(historicalAt), ShouldBeTrue)
			So(measurement.At.Equal(at.Add(10*time.Second)), ShouldBeTrue)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 600)
			_, hasRate := measurement.Metrics["liquidation_notional_rate"]
			So(hasRate, ShouldBeFalse)
			_, hasVelocity := measurement.Metrics["liquidation_share_velocity"]
			So(hasVelocity, ShouldBeFalse)

			resumed := entity.Step(tradeRow("PF_XBTUSD", 100, 1, "sell", "liquidation", at.Add(20*time.Second)))
			So(resumed.From.Equal(historicalAt), ShouldBeTrue)
			So(resumed.At.Equal(at.Add(20*time.Second)), ShouldBeTrue)
			// 700 notional across the full retained interval [-30s, +20s].
			So(resumed.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 14)
		})

		Convey("a later in-order trade still advances the clock normally", func() {
			entity.Step(tradeRow("PF_XBTUSD", 100, 3, "buy", "liquidation", at.Add(5*time.Second)))

			measurement := entity.Step(tradeRow("PF_XBTUSD", 100, 1, "sell", "liquidation", at.Add(20*time.Second)))

			So(measurement.Err, ShouldBeNil)
			// The interval now runs the full 20s from the origin.
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 35.0, 1e-9)
		})
	})
}

func TestTradeStep_SyntheticTimestamp(t *testing.T) {
	Convey("Given a payload that carried no server timestamp", t, func() {
		entity := NewTrade(t.Context())
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(tradeRow(
			"PF_XBTUSD", 100, 2, "buy", "liquidation", at.Add(time.Hour),
		)).Err, ShouldBeNil)

		Convey("its fabricated clock is folded forward instead of read as late", func() {
			// The wall-clock substitute reads as older than the exchange time,
			// but it holds no truth, so it is pinned to the timeline head and
			// the event counts as the newest observation.
			measurement := entity.Step(syntheticTradeRow("PF_XBTUSD", 100, 1, "sell", "liquidation", at))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 300.0)
		})
	})
}

func TestTradeStep_PerSymbolTimeline(t *testing.T) {
	Convey("Given trades on two symbols", t, func() {
		entity := NewTrade(t.Context())
		at := time.Unix(1_700_000_000, 0)

		So(entity.Step(tradeRow(
			"PF_XBTUSD", 100, 2, "buy", "liquidation", at.Add(time.Hour),
		)).Err, ShouldBeNil)

		Convey("each symbol keeps its own timeline", func() {
			So(entity.Step(tradeRow("PF_RAREUSD", 100, 2, "buy", "liquidation", at)).Err, ShouldBeNil)

			measurement := entity.Step(tradeRow(
				"PF_RAREUSD", 100, 1, "sell", "liquidation", at.Add(time.Second),
			))

			So(measurement.Err, ShouldBeNil)
			// The second symbol advanced normally: its rate is defined.
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 300.0, 1e-9)
		})
	})
}

/*
TestTradeRegister proves the declared schema: every producible metric is
declared, none valued, and every label names itself.
*/
func TestTradeRegister(t *testing.T) {
	Convey("Given a Trade entity", t, func() {
		entity := new(Trade)

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "gross_liquidation_notional")
			So(measurement.Metrics, ShouldContainKey, "liquidation_share")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

/*
BenchmarkTradeStep isolates the intrinsic cost of one liquidation accounting
Step over a live buy/sell leg sequence interleaved with historical reconnect
trades.
*/
func BenchmarkTradeStep(b *testing.B) {
	entity := NewTrade(b.Context())
	at := time.Unix(1_700_000_000, 0)
	// Live buy/sell legs interleaved with historical reconnect trades.
	sequence := []*data.Measurement[float64]{
		tradeRow("PF_XBTUSD", 100, 2, "buy", "liquidation", at),
		tradeRow("PF_XBTUSD", 110, 1, "sell", "trade", at.Add(10*time.Second)),
		tradeRow("PF_XBTUSD", 90, 3, "sell", "liquidation", at.Add(-30*time.Second)),
		tradeRow("PF_XBTUSD", 105, 1, "buy", "trade", at.Add(20*time.Second)),
	}
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		for _, original := range sequence {
			point := *original
			point.At = point.At.Add(time.Duration(index) * time.Minute)
			point.Metrics = maps.Clone(original.Metrics)
			measurement := entity.Step(&point)

			if measurement.Err != nil {
				b.Fatal(measurement.Err)
			}
		}
	}
}
