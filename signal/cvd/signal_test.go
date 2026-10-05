package cvd_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/cvd"
)

func TestSignalStep(t *testing.T) {
	Convey("CVD is a per-symbol Adapter algebra over executed flow", t, func() {
		arena := data.NewArenaOwner("cvd-test", 4096)
		instrument := cvd.NewSignal(context.Background(), arena)
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		read := func(measurement *data.Measurement, key string) (float64, bool) {
			entry := measurement.Read(key)

			if entry.Err != nil {
				return 0, false
			}

			return entry.Metric.Raw, true
		}

		trade := func(symbol string, step int, side float64, price float64, qty float64) *data.Measurement {
			at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
			measurement := data.NewMeasurement(1, symbol, "test:trade", int64(step+1), int64(step+1))
			measurement.At = at
			measurement.From = at
			measurement.Write(
				data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("aggressor_sign", side, data.UnitDimensionless, data.TimescaleInstantaneous),
				data.NewMetric("event_time", float64(at.UnixNano())/float64(time.Second), data.UnitSecond, data.TimescaleInstantaneous),
				data.NewMetric("best_bid", price-5, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask", price+5, data.UnitPrice, data.TimescaleInstantaneous),
			)
			return measurement
		}

		Convey("it accumulates counts, quantities, notionals and rates without signal-side arithmetic", func() {
			var buyQuantity, sellQuantity float64
			var buyNotional, sellNotional float64
			var buyCount, sellCount float64

			for step := range 10 {
				price := 50000.0 + float64(step)*10
				quantity := 1.5 + float64(step)*0.1
				side := 1.0

				if step%2 != 0 {
					side = -1
				}

				if side > 0 {
					buyCount++
					buyQuantity += quantity
					buyNotional += price * quantity
				}

				if side < 0 {
					sellCount++
					sellQuantity += quantity
					sellNotional += price * quantity
				}

				result := instrument.Step(trade("BTC/USD", step, side, price, quantity))
				So(result, ShouldNotBeNil)

				totalCount := buyCount + sellCount
				grossQuantity := buyQuantity + sellQuantity
				netQuantity := buyQuantity - sellQuantity
				grossNotional := buyNotional + sellNotional
				netNotional := buyNotional - sellNotional

				value, ok := read(result, "trade_count")
				So(ok, ShouldBeTrue)
				So(value, ShouldEqual, totalCount)

				value, ok = read(result, "trade_count:buy")
				So(ok, ShouldBeTrue)
				So(value, ShouldEqual, buyCount)

				value, ok = read(result, "trade_count:sell")
				So(ok, ShouldBeTrue)
				So(value, ShouldEqual, sellCount)

				value, ok = read(result, "executed_quantity:buy")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, buyQuantity, 1e-9)

				value, ok = read(result, "executed_quantity:sell")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, sellQuantity, 1e-9)

				value, ok = read(result, "gross_executed_quantity")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, grossQuantity, 1e-9)

				value, ok = read(result, "net_executed_quantity")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netQuantity, 1e-9)

				value, ok = read(result, "cumulative_volume_delta")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netQuantity, 1e-9)

				value, ok = read(result, "aggressive_notional:buy")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, buyNotional, 1e-6)

				value, ok = read(result, "aggressive_notional:sell")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, sellNotional, 1e-6)

				value, ok = read(result, "gross_notional")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, grossNotional, 1e-6)

				value, ok = read(result, "net_notional")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netNotional, 1e-6)

				value, ok = read(result, "cumulative_notional_delta")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netNotional, 1e-6)

				value, ok = read(result, "signed_count_fraction")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, (buyCount-sellCount)/totalCount, 1e-9)

				value, ok = read(result, "signed_net_fraction")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netNotional/grossNotional, 1e-9)

				span := float64(step) * 0.1

				if span == 0 {
					_, ok = read(result, "trade_rate")
					So(ok, ShouldBeFalse)
					continue
				}

				value, ok = read(result, "trade_rate")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, totalCount/span, 1e-6)

				value, ok = read(result, "gross_notional_rate")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, grossNotional/span, 1e-6)

				value, ok = read(result, "net_notional_rate")
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, netNotional/span, 1e-6)
			}
		})

		Convey("it isolates every stateful primitive by symbol", func() {
			firstBTC := instrument.Step(trade("BTC/USD", 0, 1, 50000, 1))
			firstETH := instrument.Step(trade("ETH/USD", 0, -1, 2000, 2))
			secondBTC := instrument.Step(trade("BTC/USD", 1, 1, 50010, 1))
			secondETH := instrument.Step(trade("ETH/USD", 1, -1, 2010, 2))

			value, ok := read(firstBTC, "trade_count")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 1)

			value, ok = read(firstETH, "trade_count")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 1)

			value, ok = read(secondBTC, "trade_count")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 2)

			value, ok = read(secondETH, "trade_count")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 2)

			value, ok = read(secondBTC, "cumulative_volume_delta")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 2)

			value, ok = read(secondETH, "cumulative_volume_delta")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, -4)
		})
	})
}
