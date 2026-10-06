package liquidity_test

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/liquidity"
)

func ticker(label string, at time.Time, seq int64, bid, ask, bidQty, askQty float64) *data.Measurement {
	prior := data.NewMeasurement(1, label, "ingress", seq, seq, data.StringEntry{Key: "channel", Value: "ticker"})
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("bid", bid, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("ask", ask, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("bid_qty", bidQty, data.UnitQuantity, data.TimescaleInstantaneous),
		data.NewMetric("ask_qty", askQty, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw, true
	}

	return 0, false
}

func TestLiquiditySignalMetrics(t *testing.T) {
	Convey("Given a READY liquidity signal", t, func() {
		instrument := liquidity.NewSignal(context.Background(), data.NewArenaOwner("liquidity", 4096))
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It measures exact touch geometry, notional depth, and imbalance", func() {
			for step := range 10 {
				at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
				bid := 3000.0 + float64(step)*2.0
				ask := bid + 4.0
				bidQty := 5.0 + float64(step)*0.5
				askQty := 3.0 + float64(step)*0.2

				res := instrument.Step(ticker("ETH/USD", at, int64(step+1), bid, ask, bidQty, askQty))
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "liquidity")
				So(res.Label, ShouldEqual, "ETH/USD")
				So(res.At, ShouldEqual, at)

				midpoint := (bid + ask) / 2.0
				spread := ask - bid
				bidNotional := bid * bidQty
				askNotional := ask * askQty
				total := bidNotional + askNotional

				expected := map[string]float64{
					"best_bid_price":           bid,
					"best_ask_price":           ask,
					"touch_quantity:bid":       bidQty,
					"touch_quantity:ask":       askQty,
					"touch_notional:bid":       bidNotional,
					"touch_notional:ask":       askNotional,
					"midpoint":                 midpoint,
					"spread":                   spread,
					"relative_spread":          spread / midpoint,
					"two_sided_touch_notional": math.Min(bidNotional, askNotional),
					"touch_notional_imbalance": (bidNotional - askNotional) / total,
				}

				for label, want := range expected {
					got, held := metric(res, label)
					So(held, ShouldBeTrue)
					So(got, ShouldAlmostEqual, want, 1e-9*math.Max(1, math.Abs(want)))
				}

				for _, channel := range [][3]string{
					{"touch_notional:bid", "touch_notional_baseline:bid", "depth_divergence:bid"},
					{"touch_notional:ask", "touch_notional_baseline:ask", "depth_divergence:ask"},
					{"relative_spread", "relative_spread_baseline", "spread_divergence"},
				} {
					value, _ := metric(res, channel[0])
					baseline, held := metric(res, channel[1])
					So(held, ShouldBeTrue)
					divergence, held := metric(res, channel[2])
					So(held, ShouldBeTrue)
					So(divergence, ShouldAlmostEqual, value-baseline, 1e-9*math.Max(1, math.Abs(value)))
				}

				ratio, held := metric(res, "depth_ratio:bid")
				So(held, ShouldBeTrue)
				baseline, _ := metric(res, "touch_notional_baseline:bid")
				So(ratio, ShouldAlmostEqual, bidNotional/baseline, 1e-9)

				_, held = metric(res, "divergence_velocity:bid")
				So(held, ShouldEqual, step > 0)
				_, held = metric(res, "spread_divergence_velocity")
				So(held, ShouldEqual, step > 0)

				if step > 1 {
					zscore, held := metric(res, "depth_zscore:bid")
					So(held, ShouldBeTrue)
					scale, held := metric(res, "depth_noise_scale:bid")
					So(held, ShouldBeTrue)
					divergence, _ := metric(res, "depth_divergence:bid")
					So(zscore, ShouldAlmostEqual, divergence/scale, 1e-9)
				}
			}
		})

		Convey("It keeps per-symbol baselines independent", func() {
			first := instrument.Step(ticker("ETH/USD", origin, 1, 3000, 3004, 5, 3))
			So(first, ShouldNotBeNil)
			other := instrument.Step(ticker("BTC/USD", origin, 2, 50000, 50010, 1, 1))
			So(other, ShouldNotBeNil)

			baseline, held := metric(other, "touch_notional_baseline:bid")
			So(held, ShouldBeTrue)
			So(baseline, ShouldAlmostEqual, 50000.0, 1e-9)

			_, held = metric(other, "divergence_velocity:bid")
			So(held, ShouldBeFalse)
		})

		Convey("It yields no measurement for a crossed touch and stays healthy", func() {
			So(instrument.Step(ticker("ETH/USD", origin, 100, 3005, 3000, 1, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)

			res := instrument.Step(ticker("ETH/USD", origin.Add(time.Second), 101, 3000, 3004, 1, 1))
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It yields no measurement for a non-positive displayed quantity", func() {
			So(instrument.Step(ticker("ETH/USD", origin, 200, 3000, 3004, 0, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It drops events before READY", func() {
			cold := liquidity.NewSignal(context.Background(), data.NewArenaOwner("liquidity", 16))
			So(cold.Step(ticker("ETH/USD", origin, 1, 3000, 3004, 1, 1)), ShouldBeNil)
		})
	})
}
