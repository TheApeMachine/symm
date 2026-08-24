package pumpdump

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func spotTicker(symbol string, bid float64, ask float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		Ask:       decimal.NewFromFloat64(ask),
		Timestamp: at,
	}
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid executable touch", t, func() {
		entity := NewTicker()
		at := time.Unix(1_700_000_000, 0)

		Convey("the first data point yields the touch and its own baseline", func() {
			measurement := entity.Step(spotTicker("BTC/USD", 99, 101, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			// The baseline of one value is the value itself: ratio one, zero divergence.
			So(measurement.Metrics["relative_spread_baseline"].Raw, ShouldAlmostEqual, 0.02, 1e-9)
			So(measurement.Metrics["spread_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-9)
			So(measurement.Metrics["spread_divergence"].Raw, ShouldAlmostEqual, 0.0, 1e-9)
			So(measurement.Metrics["spread_zscore"].Raw, ShouldAlmostEqual, 0.0, 1e-9)

			// One retained estimator sample is still immature.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a narrower follow-up touch is measured below its baseline", func() {
			entity.Step(spotTicker("BTC/USD", 99, 101, at))
			measurement := entity.Step(spotTicker("BTC/USD", 99.5, 100.5, at.Add(10*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.01, 1e-12)
			So(measurement.Metrics["spread_ratio"].Raw, ShouldBeLessThan, 1.0)
			So(measurement.Metrics["spread_divergence"].Raw, ShouldBeLessThan, 0.0)
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(spotTicker("BTC/USD", 101, 99, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
