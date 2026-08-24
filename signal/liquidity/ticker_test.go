package liquidity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func ticker(symbol string, bid, ask float64, bidQty, askQty float64) kraken.TickerData {
	bidDec := decimal.NewFromFloat64(bid)
	askDec := decimal.NewFromFloat64(ask)

	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       bidDec,
		Ask:       askDec,
		BidQty:    bidQty,
		AskQty:    askQty,
		Timestamp: time.Unix(1_700_000_000, 0),
	}
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid touch snapshot", t, func() {
		entity := NewTicker()

		Convey("Step produces exactly one measurement with no warmup", func() {
			measurement := entity.Step(ticker("BTC/USD", 99, 101, 1.0, 1.0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)
			So(measurement.Metrics["touch_notional:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["touch_notional:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_notional_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["two_sided_touch_notional"].Raw, ShouldEqual, 99.0)

			// Stateless direct measurement is whole (Maturity 1); no noise model
			// means SNR is undefined (0), derived — not caller-set.
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(ticker("BTC/USD", 101, 99, 1.0, 1.0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
