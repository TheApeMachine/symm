package cvd_test

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/cvd"
)

func TestCVDSignalMetrics(t *testing.T) {
	Convey("CVD composes executed-flow accounting without leaving the primitive algebra", t, func() {
		arena := data.NewArenaOwner("cvd-test", 4096)
		instrument := cvd.NewSignal(context.Background(), arena)
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		var buyQty, sellQty, buyNotional, sellNotional, buyCount, sellCount float64

		for step := range 10 {
			at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
			price := 50000.0 + float64(step)*10.0
			qty := 1.5 + float64(step)*0.1
			side := 1.0

			if step%2 != 0 {
				side = -1
				sellCount++
				sellQty += qty
				sellNotional += price * qty
			}

			if side > 0 {
				buyCount++
				buyQty += qty
				buyNotional += price * qty
			}

			prior := data.NewMeasurement(1, "BTC/USD", "ingress", int64(step+1), int64(step+1))
			prior.At = at
			prior.From = at
			prior.Write(
				data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("side", side, data.UnitDimensionless, data.TimescaleInstantaneous),
				data.NewMetric("best_bid", price-5, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask", price+5, data.UnitPrice, data.TimescaleInstantaneous),
			)

			result := instrument.Step(prior)
			So(result, ShouldNotBeNil)

			total := float64(step + 1)
			grossQty := buyQty + sellQty
			netQty := buyQty - sellQty
			grossNotional := buyNotional + sellNotional
			netNotional := buyNotional - sellNotional

			So(result.Read("trade_count").Metric.Raw, ShouldEqual, total)
			So(result.Read("trade_count:buy").Metric.Raw, ShouldEqual, buyCount)
			So(result.Read("trade_count:sell").Metric.Raw, ShouldEqual, sellCount)
			So(result.Read("signed_count_fraction").Metric.Raw, ShouldAlmostEqual, (buyCount-sellCount)/total, 1e-12)

			So(result.Read("executed_quantity:buy").Metric.Raw, ShouldAlmostEqual, buyQty, 1e-12)
			So(result.Read("executed_quantity:sell").Metric.Raw, ShouldAlmostEqual, sellQty, 1e-12)
			So(result.Read("gross_executed_quantity").Metric.Raw, ShouldAlmostEqual, grossQty, 1e-12)
			So(result.Read("net_executed_quantity").Metric.Raw, ShouldAlmostEqual, netQty, 1e-12)
			So(result.Read("cumulative_volume_delta").Metric.Raw, ShouldAlmostEqual, netQty, 1e-12)

			So(result.Read("aggressive_notional:buy").Metric.Raw, ShouldAlmostEqual, buyNotional, 1e-9)
			So(result.Read("aggressive_notional:sell").Metric.Raw, ShouldAlmostEqual, sellNotional, 1e-9)
			So(result.Read("gross_notional").Metric.Raw, ShouldAlmostEqual, grossNotional, 1e-9)
			So(result.Read("net_notional").Metric.Raw, ShouldAlmostEqual, netNotional, 1e-9)
			So(result.Read("cumulative_notional_delta").Metric.Raw, ShouldAlmostEqual, netNotional, 1e-9)
			So(result.Read("signed_net_fraction").Metric.Raw, ShouldAlmostEqual, netNotional/grossNotional, 1e-12)
			So(result.Read("mean_trade_notional").Metric.Raw, ShouldAlmostEqual, grossNotional/total, 1e-9)

			So(result.Read("response_midpoint:from").Metric.Raw, ShouldAlmostEqual, 50000.0, 1e-12)
			So(result.Read("response_midpoint:at").Metric.Raw, ShouldAlmostEqual, price, 1e-12)
			So(result.Read("midpoint_log_return").Metric.Raw, ShouldAlmostEqual, math.Log(price/50000.0), 1e-12)

			elapsed := at.Sub(origin).Seconds()

			if elapsed == 0 {
				So(result.Read("trade_rate").Err, ShouldNotBeNil)
				So(result.Read("midpoint_return_rate").Err, ShouldNotBeNil)
				continue
			}

			So(result.Read("trade_rate").Metric.Raw, ShouldAlmostEqual, total/elapsed, 1e-9)
			So(result.Read("gross_notional_rate").Metric.Raw, ShouldAlmostEqual, grossNotional/elapsed, 1e-6)
			So(result.Read("net_notional_rate").Metric.Raw, ShouldAlmostEqual, netNotional/elapsed, 1e-6)
			So(result.Read("buy_notional_rate").Metric.Raw, ShouldAlmostEqual, buyNotional/elapsed, 1e-6)
			So(result.Read("sell_notional_rate").Metric.Raw, ShouldAlmostEqual, sellNotional/elapsed, 1e-6)
			So(result.Read("midpoint_return_rate").Metric.Raw, ShouldAlmostEqual, math.Log(price/50000.0)/elapsed, 1e-12)

			if step >= 3 {
				So(result.Read("net_notional_rate_velocity").Err, ShouldBeNil)
				So(result.Read("gross_notional_rate_velocity").Err, ShouldBeNil)
			}
		}
	})

	Convey("CVD keeps every symbol's primitive state independent", t, func() {
		arena := data.NewArenaOwner("cvd-symbol-test", 4096)
		instrument := cvd.NewSignal(context.Background(), arena)
		instrument.Transition(nmruntime.READY)
		at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		btc := data.NewMeasurement(1, "BTC/USD", "ingress", 1, 1)
		btc.At = at
		btc.From = at
		btc.Write(
			data.NewMetric("price", 100, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			data.NewMetric("side", 1, data.UnitDimensionless, data.TimescaleInstantaneous),
			data.NewMetric("best_bid", 99, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("best_ask", 101, data.UnitPrice, data.TimescaleInstantaneous),
		)

		eth := data.NewMeasurement(1, "ETH/USD", "ingress", 2, 2)
		eth.At = at
		eth.From = at
		eth.Write(
			data.NewMetric("price", 200, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 2, data.UnitQuantity, data.TimescaleInstantaneous),
			data.NewMetric("side", -1, data.UnitDimensionless, data.TimescaleInstantaneous),
			data.NewMetric("best_bid", 199, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("best_ask", 201, data.UnitPrice, data.TimescaleInstantaneous),
		)

		So(instrument.Step(btc).Read("trade_count").Metric.Raw, ShouldEqual, 1.0)
		ethResult := instrument.Step(eth)
		So(ethResult.Read("trade_count").Metric.Raw, ShouldEqual, 1.0)
		So(ethResult.Read("trade_count:buy").Metric.Raw, ShouldEqual, 0.0)
		So(ethResult.Read("trade_count:sell").Metric.Raw, ShouldEqual, 1.0)

		btc2 := data.NewMeasurement(1, "BTC/USD", "ingress", 3, 3)
		btc2.At = at.Add(time.Second)
		btc2.From = btc2.At
		btc2.Write(
			data.NewMetric("price", 101, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			data.NewMetric("side", 1, data.UnitDimensionless, data.TimescaleInstantaneous),
			data.NewMetric("best_bid", 100, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("best_ask", 102, data.UnitPrice, data.TimescaleInstantaneous),
		)

		btcResult := instrument.Step(btc2)
		So(btcResult.Read("trade_count").Metric.Raw, ShouldEqual, 2.0)
		So(btcResult.Read("trade_count:buy").Metric.Raw, ShouldEqual, 2.0)
		So(btcResult.Read("trade_count:sell").Metric.Raw, ShouldEqual, 0.0)
	})

	Convey("CVD rejects non-positive trade coordinates and non-unit aggressor signs", t, func() {
		for _, values := range [][3]float64{
			{0, 1, 1},
			{100, 0, 1},
			{100, 1, 0},
			{100, 1, 2},
		} {
			arena := data.NewArenaOwner("cvd-domain-test", 64)
			instrument := cvd.NewSignal(context.Background(), arena)
			instrument.Transition(nmruntime.READY)
			prior := data.NewMeasurement(1, "BTC/USD", "ingress", 1, 1)
			prior.At = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			prior.From = prior.At
			prior.Write(
				data.NewMetric("price", values[0], data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", values[1], data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("side", values[2], data.UnitDimensionless, data.TimescaleInstantaneous),
				data.NewMetric("best_bid", 99, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask", 101, data.UnitPrice, data.TimescaleInstantaneous),
			)

			So(instrument.Step(prior), ShouldBeNil)
		}
	})
}
