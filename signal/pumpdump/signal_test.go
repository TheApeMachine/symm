package pumpdump_test

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
	"github.com/theapemachine/symm/signal/pumpdump"
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

func TestPumpDumpSignal(t *testing.T) {
	Convey("Volume-clocked activity instrument measures tape, touch, and midpoint response", t, func() {
		ctx := context.Background()
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		books := broker.NewBook(ctx, spot.NewNormalizer())

		instrument := pumpdump.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)

		Convey("Volume clock aggregates exact trade bars, durations, and rates", func() {
			So(market.Quote(books, "BTC/USD", now, 50000.0, 5.0, 50002.0, 5.0), ShouldBeNil)

			res1 := instrument.Step(market.Stamp(books, "BTC/USD", "buy", now, 50001.0, 1.0))
			So(res1, ShouldNotBeNil)

			// The first trade only seeds the quantity distribution: no
			// interval, no bar, no rates.
			for _, label := range []string{
				"volume_rate", "trade_rate", "positive_midpoint_return", "negative_midpoint_return",
			} {
				_, held := metric(res1, label)
				So(held, ShouldBeFalse)
			}

			at2 := now.Add(200 * time.Millisecond)
			res2 := instrument.Step(market.Stamp(books, "BTC/USD", "buy", at2, 50002.0, 1.0))
			So(res2, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			input3 := market.Stamp(books, "BTC/USD", "buy", now.Add(500*time.Millisecond), 50003.0, 1.0)
			res3 := instrument.Step(input3)
			So(res3, ShouldNotBeNil)

			So(metricValue(res3, "volume_rate"), ShouldAlmostEqual, 2.0/0.3, 1e-6)
			So(metricValue(res3, "trade_rate"), ShouldAlmostEqual, 2.0/0.3, 1e-6)
			So(metricValue(res3, "positive_midpoint_return"), ShouldEqual, 0.0)
			So(metricValue(res3, "negative_midpoint_return"), ShouldEqual, 0.0)
		})

		Convey("Pump: activity surge and spread blowout yield spread_ratio and positive return", func() {
			basePrice := 50000.0

			for step := range 12 {
				at := now.Add(time.Duration(step*100) * time.Millisecond)
				half := 1.0 + 0.1*float64(step%2)
				So(market.Quote(books, "BTC/USD", at, basePrice-half, 5.0, basePrice+half, 5.0), ShouldBeNil)
				So(instrument.Step(market.Stamp(books, "BTC/USD", "buy", at, basePrice, 0.5)), ShouldNotBeNil)
			}

			pumpAt := now.Add(1300 * time.Millisecond)
			So(market.Quote(books, "BTC/USD", pumpAt, basePrice, 1.0, basePrice+40.0, 1.0), ShouldBeNil)

			pumpRes := instrument.Step(market.Stamp(books, "BTC/USD", "buy", pumpAt, basePrice+35.0, 10.0))
			So(pumpRes, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			So(metricValue(pumpRes, "spread_ratio"), ShouldBeGreaterThan, 1.0)
			So(metricValue(pumpRes, "positive_midpoint_return"), ShouldBeGreaterThan, 0.0)
			So(metricValue(pumpRes, "negative_midpoint_return"), ShouldEqual, 0.0)
		})

		Convey("Dump: a midpoint crash yields negative_midpoint_return", func() {
			basePrice := 50000.0

			for step := range 5 {
				at := now.Add(time.Duration(step*100) * time.Millisecond)
				So(market.Quote(books, "BTC/USD", at, basePrice-2.0, 5.0, basePrice+2.0, 5.0), ShouldBeNil)
				So(instrument.Step(market.Stamp(books, "BTC/USD", "sell", at, basePrice, 1.0)), ShouldNotBeNil)
			}

			dumpAt := now.Add(600 * time.Millisecond)
			So(market.Quote(books, "BTC/USD", dumpAt, basePrice-52.0, 5.0, basePrice-48.0, 5.0), ShouldBeNil)

			dumpRes := instrument.Step(market.Stamp(books, "BTC/USD", "sell", dumpAt, basePrice-50.0, 3.0))
			So(dumpRes, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			logReturn := math.Log((basePrice - 50.0) / basePrice)
			So(metricValue(dumpRes, "positive_midpoint_return"), ShouldEqual, 0.0)
			So(metricValue(dumpRes, "negative_midpoint_return"), ShouldAlmostEqual, -logReturn, 1e-12)
		})

		Convey("A stamped book that cannot be decoded is corrupt ingress state and halts", func() {
			So(instrument.Step(market.Unreadable("BTC/USD", "buy", now, 50000.0, 1.0)), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "stamped book unreadable")
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})

		Convey("A trade without a positive quantity yields no measurement", func() {
			So(market.Quote(books, "BTC/USD", now, 50000.0, 5.0, 50002.0, 5.0), ShouldBeNil)
			So(instrument.Step(market.Stamp(books, "BTC/USD", "buy", now, 50001.0, 0.0)), ShouldBeNil)
		})

		Convey("A trade frame without price is an error, not a silent drop", func() {
			prior := data.NewMeasurement(
				1, "BTC/USD", "spot:trade", 90, 90,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			prior.At = now
			prior.From = now

			So(instrument.Step(prior.Write(
				data.NewMetric("qty", 1.0, data.UnitQuantity, data.TimescaleInstantaneous),
			)), ShouldBeNil)
			So(instrument.Error(), ShouldNotBeNil)
		})
	})
}
