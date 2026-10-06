package liquidity_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/liquidity"
)

/*
trade builds a spot trade frame, the only frame the pipeline carries. It holds
trade fields only; the touch lives in the book manager.
*/
func trade(label string, at time.Time, seq int64, price, qty float64) *data.Measurement {
	prior := data.NewMeasurement(
		1, label, "spot:trade", seq, seq,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: "buy"},
	)
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

/*
touch seeds the book manager with a one-level-per-side Level3 snapshot.
*/
func touch(books *broker.Book, label string, at time.Time, bid, ask, bidQty, askQty float64) {
	books.Update(&kraken.Level3{
		Channel: "level3",
		Type:    "snapshot",
		Data: []kraken.Level3Data{{
			Symbol: label,
			Bids: []kraken.Level3Order{{
				OrderID: label + "-bid", LimitPrice: decimal.NewFromFloat64(bid), OrderQty: decimal.NewFromFloat64(bidQty),
				Timestamp: at, Event: "add",
			}},
			Asks: []kraken.Level3Order{{
				OrderID: label + "-ask", LimitPrice: decimal.NewFromFloat64(ask), OrderQty: decimal.NewFromFloat64(askQty),
				Timestamp: at, Event: "add",
			}},
		}},
	})
}

/*
stepTouch seeds the touch in the book manager, then drives the signal with a trade
frame at the midpoint.
*/
func stepTouch(
	instrument *liquidity.Signal, books *broker.Book,
	label string, at time.Time, seq int64, bid, ask, bidQty, askQty float64,
) *data.Measurement {
	touch(books, label, at, bid, ask, bidQty, askQty)
	return instrument.Step(trade(label, at, seq, (bid+ask)/2, 1))
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw, true
	}

	return 0, false
}

func TestLiquiditySignalMetrics(t *testing.T) {
	Convey("Given a READY liquidity signal", t, func() {
		ctx := context.Background()
		books := broker.NewBook(ctx, spot.NewNormalizer())
		instrument := liquidity.NewSignal(ctx, data.NewArenaOwner("liquidity", 4096), books)
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It measures exact touch geometry, notional depth, and imbalance", func() {
			for step := range 10 {
				at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
				bid := 3000.0 + float64(step)*2.0
				ask := bid + 4.0
				bidQty := 5.0 + float64(step)*0.5
				askQty := 3.0 + float64(step)*0.2

				res := stepTouch(instrument, books, "ETH/USD", at, int64(step+1), bid, ask, bidQty, askQty)
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "liquidity")
				So(res.Label, ShouldEqual, "ETH/USD")
				So(res.At, ShouldEqual, at)

				if step == 0 {
					for _, label := range []string{
						"depth_noise_scale:bid", "depth_noise_scale:ask", "spread_noise_scale",
						"depth_zscore:bid", "depth_zscore:ask", "spread_zscore",
						"divergence_velocity:bid", "divergence_velocity:ask", "spread_divergence_velocity",
						"historical_path_distance", "historical_path_percentile",
					} {
						val, held := metric(res, label)
						So(held, ShouldBeTrue)
						So(val, ShouldEqual, 0.0)
					}
				}

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
				So(held, ShouldBeTrue)
				_, held = metric(res, "spread_divergence_velocity")
				So(held, ShouldBeTrue)

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
			first := stepTouch(instrument, books, "ETH/USD", origin, 1, 3000, 3004, 5, 3)
			So(first, ShouldNotBeNil)
			other := stepTouch(instrument, books, "BTC/USD", origin, 2, 50000, 50010, 1, 1)
			So(other, ShouldNotBeNil)

			baseline, held := metric(other, "touch_notional_baseline:bid")
			So(held, ShouldBeTrue)
			So(baseline, ShouldAlmostEqual, 50000.0, 1e-9)

			otherVel, held := metric(other, "divergence_velocity:bid")
			So(held, ShouldBeTrue)
			So(otherVel, ShouldEqual, 0.0)
		})

		Convey("A crossed touch is corrupt book state: it halts with an Internal error naming the symbol", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 100, 3005, 3000, 1, 1), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "crossed or locked")
			So(err.Error(), ShouldContainSubstring, "ETH/USD")
			So(strings.Contains(err.Error(), "book manager is required"), ShouldBeFalse)
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)

			// A halted signal never resumes on a later healthy touch.
			So(stepTouch(instrument, books, "ETH/USD", origin.Add(time.Second), 101, 3000, 3004, 1, 1), ShouldBeNil)
		})

		Convey("A locked touch (ask equal to bid) halts the same way", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 110, 3000, 3000, 1, 1), ShouldBeNil)
			So(errnie.IsInternal(instrument.Error()), ShouldBeTrue)
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})

		Convey("A present touch with a non-positive price is corrupt book state and halts", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 120, -1, 3004, 1, 1), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "non-finite or non-positive book touch")
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})

		Convey("It yields no measurement for a non-positive displayed quantity", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 200, 3000, 3004, 0, 1), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It drops events before READY", func() {
			cold := liquidity.NewSignal(ctx, data.NewArenaOwner("liquidity", 16), books)
			So(stepTouch(cold, books, "ETH/USD", origin, 1, 3000, 3004, 1, 1), ShouldBeNil)
		})

		Convey("It reads the touch only from the book manager, never from the trade frame", func() {
			touch(books, "ETH/USD", origin, 3000, 3004, 5, 3)

			prior := data.NewMeasurement(1, "ETH/USD", "spot:trade", 300, 300)
			prior.At = origin
			prior.From = origin
			prior = prior.Write(
				data.NewMetric("price", 3002, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("bid", 1, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("ask", 2, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("bid_qty", 99, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("ask_qty", 99, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			for label, want := range map[string]float64{
				"best_bid_price": 3000, "best_ask_price": 3004,
				"touch_quantity:bid": 5, "touch_quantity:ask": 3,
			} {
				got, held := metric(res, label)
				So(held, ShouldBeTrue)
				So(got, ShouldEqual, want)
			}
		})

		Convey("It yields no measurement while the symbol has no book and stays healthy", func() {
			So(instrument.Step(trade("SOL/USD", origin, 400, 150, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})
	})

	Convey("Given a liquidity signal constructed without a book manager", t, func() {
		instrument := liquidity.NewSignal(context.Background(), data.NewArenaOwner("liquidity", 16), nil)

		Convey("It fails construction with an error instead of running blind", func() {
			So(instrument.Error(), ShouldNotBeNil)
			So(instrument.Status(), ShouldNotEqual, nmruntime.READY)
		})
	})
}
