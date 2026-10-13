package toxicity_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/tests/market"
)

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw(), true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func TestToxicitySignal(t *testing.T) {
	Convey("Toxicity instrument calculates exact touch dispositions and trade fill matching", t, func() {
		ctx := context.Background()
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		books := broker.NewBook(ctx, spot.NewNormalizer())

		instrument := toxicity.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)

		Convey("A retreat removes only the unfilled residual", func() {
			So(market.Quote(books, "BTC/USD", now, 50000.0, 10.0, 50002.0, 2.0), ShouldBeNil)
			So(instrument.Step(market.Stamp(books, "BTC/USD", "buy", now, 50001.0, 1.0)), ShouldNotBeNil)

			So(instrument.Step(market.Stamp(books, "BTC/USD", "sell", now, 50000.0, 4.0)), ShouldNotBeNil)

			later := now.Add(200 * time.Millisecond)
			So(market.Quote(books, "BTC/USD", later, 49990.0, 8.0, 50003.0, 1.0), ShouldBeNil)

			res := instrument.Step(market.Stamp(books, "BTC/USD", "buy", later, 50002.0, 5.0))
			So(res, ShouldNotBeNil)

			// Bid: E = 4 of Q0 = 10, so R = U = 6. retreat_fraction:bid = 6/10 = 0.6
			So(metricValue(res, "retreat_fraction:bid"), ShouldAlmostEqual, 0.6, 1e-9)
		})

		Convey("Touch disposition measures withdrawal at a held price", func() {
			So(market.Quote(books, "BTC/USD", now, 50000.0, 10.0, 50002.0, 4.0), ShouldBeNil)
			So(instrument.Step(market.Stamp(books, "BTC/USD", "buy", now, 50001.0, 1.0)), ShouldNotBeNil)

			later := now.Add(500 * time.Millisecond)
			So(market.Quote(books, "BTC/USD", later, 50000.0, 6.0, 50002.0, 7.0), ShouldBeNil)

			res := instrument.Step(market.Stamp(books, "BTC/USD", "buy", later, 50001.0, 1.0))
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			// 10 - 6 = 4 withdrawn over 0.5s = 8/s
			So(metricValue(res, "net_withdrawal_rate:bid"), ShouldAlmostEqual, 4.0/0.5, 1e-6)
			So(metricValue(res, "retreat_fraction:bid"), ShouldAlmostEqual, 0.0, 1e-9)
		})

		Convey("A crossed book leaves the trade unstamped and yields no measurement without halting", func() {
			So(market.Quote(books, "BTC/USD", now, 50010.0, 1.0, 50000.0, 1.0), ShouldBeNil)

			So(instrument.Step(market.Stamp(books, "BTC/USD", "buy", now, 50005.0, 1.0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(instrument.Status(), ShouldEqual, nmruntime.READY)
		})

		Convey("A stamped book that cannot be decoded is corrupt ingress state and halts", func() {
			So(instrument.Step(market.Unreadable("BTC/USD", "buy", now, 50002.0, 1.0)), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})
	})
}
