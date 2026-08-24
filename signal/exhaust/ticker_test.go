package exhaust

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/theapemachine/symm/kraken"
)

func ticker(symbol string, bid, ask float64, bidQty, askQty float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		Ask:       decimal.NewFromFloat64(ask),
		BidQty:    bidQty,
		AskQty:    askQty,
		Timestamp: at,
	}
}

func TestTickerStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)

	Convey("Given a valid touch snapshot", t, func() {
		entity := NewTicker()

		Convey("Step produces exactly one measurement with no warmup", func() {
			measurement := entity.Step(ticker("BTC/USD", 99, 101, 2, 2, now))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["displayed_depth_notional:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(measurement.Metrics["displayed_depth_notional:ask"].Raw, ShouldAlmostEqual, 202.0, 1e-12)
			So(measurement.Metrics["displayed_depth_notional"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(measurement.Metrics["spread"].Raw, ShouldAlmostEqual, 2.0, 1e-12)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)
			So(measurement.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["previous_book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["book_imbalance_change"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
			So(measurement.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, 0.0, 1e-12)

			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("stateful metrics advance over a multi-update sequence", func() {
			first := entity.Step(ticker("BTC/USD", 99, 101, 2, 2, now))
			So(first.Err, ShouldBeNil)

			second := entity.Step(ticker("BTC/USD", 100, 102, 4, 2, later))

			So(second, ShouldNotBeNil)
			So(second.Err, ShouldBeNil)

			So(second.Metrics["displayed_depth_notional:bid"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(second.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, 196.0/604.0, 1e-12)
			So(second.Metrics["previous_book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(second.Metrics["book_imbalance_change"].Raw, ShouldAlmostEqual, 196.0/604.0+0.01, 1e-12)
			So(second.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, math.Log(101.0/100.0), 1e-12)

			So(second.Maturity, ShouldEqual, 0.5)
			So(second.SNR, ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(ticker("BTC/USD", 101, 99, 1, 1, now))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
