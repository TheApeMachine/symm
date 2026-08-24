package cvd

import (
	"math"
	"testing"
	"time"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func cvdTrade(symbol string, side string, price float64, qty float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func cvdBook(bidPrice float64, askPrice float64) *book.Book {
	resident := book.New()
	resident.Update(&book.UpdateOptions{
		Direction: book.Bid,
		Price:     decimal.NewFromFloat64(bidPrice),
		Quantity:  decimal.NewFromFloat64(1),
		Timestamp: time.Unix(1_700_000_000, 0),
	})
	resident.Update(&book.UpdateOptions{
		Direction: book.Ask,
		Price:     decimal.NewFromFloat64(askPrice),
		Quantity:  decimal.NewFromFloat64(1),
		Timestamp: time.Unix(1_700_000_000, 0),
	})

	return resident
}

func TestTradeStep(t *testing.T) {
	Convey("Given an executed-flow entity with a shared book", t, func() {
		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", cvdBook(99, 101), "BTC/USD")
		entity := NewTrade(workspace)

		Convey("the first buy trade yields a measurement with no warmup gating", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["executed_quantity:buy"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["executed_quantity:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_executed_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["net_executed_quantity"].Raw, ShouldEqual, 2.0)

			So(measurement.Metrics["aggressive_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["aggressive_notional:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["mean_trade_notional"].Raw, ShouldEqual, 200.0)

			So(measurement.Metrics["cumulative_volume_delta"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["cumulative_notional_delta"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["cvd_epoch_from"].Raw, ShouldEqual, 1000.0)

			// One trade carries a single effective observation: Maturity 0.
			So(measurement.Maturity, ShouldEqual, 0.0)

			// Rates, velocities, baselines, and response-price metrics are
			// undefined until their prerequisites exist.
			_, hasTradeRate := measurement.Metrics["trade_rate"]
			_, hasBaseline := measurement.Metrics["signed_net_fraction_baseline"]
			_, hasMidpoint := measurement.Metrics["midpoint_log_return"]

			So(hasTradeRate, ShouldBeFalse)
			So(hasBaseline, ShouldBeFalse)
			So(hasMidpoint, ShouldBeFalse)
		})

		Convey("a second sell trade advances accounting, rates, and baselines", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			measurement := entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(measurement.Metrics["mean_trade_notional"].Raw, ShouldEqual, 150.0)

			So(measurement.Metrics["trade_rate"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["gross_notional_rate"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional_rate"].Raw, ShouldEqual, 100.0)

			// Causal directional baseline is the previous committed fraction;
			// divergence and z-score are judged against it.
			So(measurement.Metrics["signed_net_fraction_baseline"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_net_fraction_divergence"].Raw, ShouldAlmostEqual, -2.0/3.0, 1e-12)
			So(measurement.Metrics["signed_net_fraction_zscore"].Raw, ShouldEqual, -1.0)

			_, hasGrossBaseline := measurement.Metrics["gross_notional_rate_baseline"]

			So(hasGrossBaseline, ShouldBeFalse)
			So(measurement.Maturity, ShouldEqual, 0.5)
		})

		Convey("a third buy trade advances baselines, velocities, and response", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			workspace.Share("book", cvdBook(109, 111), "BTC/USD")
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 1, time.Unix(1003, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)

			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 400.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldEqual, 0.5)

			So(measurement.Metrics["gross_notional_rate"].Raw, ShouldAlmostEqual, 400.0/3.0, 1e-9)
			So(measurement.Metrics["signed_net_fraction_baseline"].Raw, ShouldAlmostEqual, 2.0/3.0, 1e-12)
			So(measurement.Metrics["signed_net_fraction_divergence"].Raw, ShouldAlmostEqual, -1.0/6.0, 1e-12)

			So(measurement.Metrics["gross_notional_rate_baseline"].Raw, ShouldAlmostEqual, 300.0, 1e-9)
			So(measurement.Metrics["gross_notional_rate_ratio"].Raw, ShouldAlmostEqual, 4.0/9.0, 1e-12)
			So(measurement.Metrics["gross_notional_rate_divergence"].Raw, ShouldAlmostEqual, math.Log(4.0/9.0), 1e-9)
			So(measurement.Metrics["gross_notional_rate_zscore"].Raw, ShouldEqual, -1.0)

			So(measurement.Metrics["net_notional_rate_velocity"].Raw, ShouldAlmostEqual, -50.0/3.0, 1e-9)
			So(measurement.Metrics["gross_notional_rate_velocity"].Raw, ShouldAlmostEqual, math.Log(4.0/9.0)/2.0, 1e-9)

			So(measurement.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, math.Log(1.1), 1e-9)
			So(measurement.Metrics["midpoint_return_rate"].Raw, ShouldAlmostEqual, math.Log(1.1)/3.0, 1e-9)
			So(measurement.Metrics["flow_aligned_midpoint_return"].Raw, ShouldAlmostEqual, math.Log(1.1), 1e-9)
			So(measurement.Metrics["midpoint_response_per_net_notional"].Raw, ShouldAlmostEqual, math.Log(1.1)/200.0, 1e-12)
			So(measurement.Metrics["midpoint_return_rate_baseline"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["midpoint_return_rate_divergence"].Raw, ShouldAlmostEqual, math.Log(1.1)/3.0, 1e-9)
			So(measurement.Metrics["midpoint_return_rate_zscore"].Raw, ShouldEqual, 1.0)
		})
	})

	Convey("Given a symbol with no shared book", t, func() {
		entity := NewTrade(runtime.NewWorkspace(nil))

		Convey("executed-flow accounting still stands and response-price metrics are absent", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			measurement := entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 2.0)

			_, hasMidpoint := measurement.Metrics["midpoint_log_return"]

			So(hasMidpoint, ShouldBeFalse)
		})
	})

	Convey("Given a non-positive execution price", t, func() {
		entity := NewTrade(nil)

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 0, 1, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
