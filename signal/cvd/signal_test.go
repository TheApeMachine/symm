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

func trade(at time.Time, seq int64, side string, price, qty float64) *data.Measurement {
	prior := data.NewMeasurement(1, "BTC/USD", "ingress", seq, seq, &data.StringEntry{Key: "side", Value: side})
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw, true
	}

	return 0, false
}

func TestCVDSignalMetrics(t *testing.T) {
	Convey("Given a READY CVD signal", t, func() {
		instrument := cvd.NewSignal(context.Background(), data.NewArenaOwner("cvd", 4096))
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It accumulates counts, quantities, notionals, fractions, and rates", func() {
			var buyQty, sellQty, buyNotional, sellNotional, buyCount, sellCount float64

			for step := range 10 {
				at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
				price := 50000.0 + float64(step)*10.0
				qty := 1.5 + float64(step)*0.1
				side := "buy"

				if step%2 != 0 {
					side = "sell"
					sellCount++
					sellQty += qty
					sellNotional += price * qty
				} else {
					buyCount++
					buyQty += qty
					buyNotional += price * qty
				}

				res := instrument.Step(trade(at, int64(step+1), side, price, qty))
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "cvd")
				So(res.Label, ShouldEqual, "BTC/USD")
				So(res.At, ShouldEqual, at)
				So(res.From, ShouldEqual, origin)

				total := buyCount + sellCount
				gross := buyNotional + sellNotional
				net := buyNotional - sellNotional

				expected := map[string]float64{
					"trade_count":               total,
					"trade_count:buy":           buyCount,
					"trade_count:sell":          sellCount,
					"signed_count_fraction":     (buyCount - sellCount) / total,
					"executed_quantity:buy":     buyQty,
					"executed_quantity:sell":    sellQty,
					"gross_executed_quantity":   buyQty + sellQty,
					"net_executed_quantity":     buyQty - sellQty,
					"cumulative_volume_delta":   buyQty - sellQty,
					"aggressive_notional:buy":   buyNotional,
					"aggressive_notional:sell":  sellNotional,
					"gross_notional":            gross,
					"net_notional":              net,
					"cumulative_notional_delta": net,
					"signed_net_fraction":       net / gross,
					"mean_trade_notional":       gross / total,
				}

				for label, want := range expected {
					got, held := metric(res, label)
					So(held, ShouldBeTrue)
					So(got, ShouldAlmostEqual, want, 1e-6*math.Max(1, math.Abs(want)))
				}

				span := at.Sub(origin).Seconds()
				rates := map[string]float64{
					"trade_rate":          total,
					"gross_notional_rate": gross,
					"net_notional_rate":   net,
					"buy_notional_rate":   buyNotional,
					"sell_notional_rate":  sellNotional,
				}

				for label, numerator := range rates {
					got, held := metric(res, label)

					if span == 0 {
						So(held, ShouldBeFalse)
						continue
					}

					So(held, ShouldBeTrue)
					So(got, ShouldAlmostEqual, numerator/span, 1e-6*math.Max(1, math.Abs(numerator/span)))
				}

				baseline, held := metric(res, "signed_net_fraction_baseline")
				So(held, ShouldBeTrue)
				divergence, held := metric(res, "signed_net_fraction_divergence")
				So(held, ShouldBeTrue)
				So(divergence, ShouldAlmostEqual, net/gross-baseline, 1e-12)
			}
		})

		Convey("It drops a trade without an explicit aggressor side", func() {
			So(instrument.Step(trade(origin, 1, "", 50000, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It drops events before READY", func() {
			cold := cvd.NewSignal(context.Background(), data.NewArenaOwner("cvd", 16))
			So(cold.Step(trade(origin, 1, "buy", 50000, 1)), ShouldBeNil)
		})
	})
}
