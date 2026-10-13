package liquidity_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/tests/market"
)

/*
stepTouch seeds the touch in the book manager, then drives the signal with a trade
frame at the midpoint.
*/
func stepTouch(
	instrument *liquidity.Signal, books *broker.Book,
	label string, at time.Time, bid, ask, bidQty, askQty float64,
) *data.Measurement {
	So(market.Quote(books, label, at, bid, bidQty, ask, askQty), ShouldBeNil)
	return instrument.Step(market.Stamp(books, label, "buy", at, (bid+ask)/2, 1))
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw(), true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func TestLiquiditySignalMetrics(t *testing.T) {
	Convey("Given a READY liquidity signal", t, func() {
		ctx := context.Background()
		books := broker.NewBook(ctx, spot.NewNormalizer())
		instrument := liquidity.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It measures exact touch geometry and relative spread", func() {
			for step := range 14 {
				at := origin.Add(time.Duration(step) * 100 * time.Millisecond)
				bid := 3000.0 + float64(step)*2.0
				ask := bid + 4.0

				res := stepTouch(instrument, books, "ETH/USD", at, bid, ask, 5.0, 3.0)
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "liquidity")
				So(res.Label, ShouldEqual, "ETH/USD")
				So(res.At, ShouldEqual, at)

				midpoint := (bid + ask) / 2.0
				spread := ask - bid

				So(metricValue(res, "best_bid_price"), ShouldEqual, bid)
				So(metricValue(res, "best_ask_price"), ShouldEqual, ask)
				So(metricValue(res, "relative_spread"), ShouldAlmostEqual, spread/midpoint, 1e-9*math.Max(1, math.Abs(spread/midpoint)))
			}
		})

		Convey("A crossed touch is dropped with a warning without halting the system", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 3005, 3000, 1, 1), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(instrument.Status(), ShouldEqual, nmruntime.READY)

			healthyRes := stepTouch(instrument, books, "ETH/USD", origin.Add(time.Second), 3000, 3004, 1, 1)
			So(healthyRes, ShouldNotBeNil)
		})

		Convey("A locked touch (ask equal to bid) is dropped without halting the system", func() {
			So(stepTouch(instrument, books, "ETH/USD", origin, 3000, 3000, 1, 1), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(instrument.Status(), ShouldEqual, nmruntime.READY)
		})

		Convey("A stamped book that cannot be decoded is corrupt ingress state and halts", func() {
			So(instrument.Step(market.Unreadable("ETH/USD", "buy", origin, 3000, 1)), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "stamped book unreadable")
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})

		Convey("It drops events before READY", func() {
			cold := liquidity.NewSignal(ctx)
			So(stepTouch(cold, books, "ETH/USD", origin, 3000, 3004, 1, 1), ShouldBeNil)
		})

		Convey("It yields no measurement while the symbol has no book and stays healthy", func() {
			So(instrument.Step(market.Stamp(books, "SOL/USD", "buy", origin, 150, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})
	})
}
