package hawkes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/hawkes"
)

func trade(at time.Time, seq int64, side string, price, qty float64) *data.Measurement {
	prior := data.NewMeasurement(
		1, "BTC/USD", "ingress", seq, seq,
		data.StringEntry{Key: "side", Value: side},
		data.StringEntry{Key: "channel", Value: "trade"},
	)
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw, true
	}

	return 0, false
}

func TestHawkesTradeMetrics(t *testing.T) {
	Convey("Given a READY Hawkes arrival-dynamics signal", t, func() {
		instrument := hawkes.NewSignal(context.Background(), data.NewArenaOwner("hawkes", 4096))
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It computes exact event counts, side fractions, and arrival rates", func() {
			var buyCount, sellCount float64

			for step := range 20 {
				at := origin.Add(time.Duration(step) * 50 * time.Millisecond)
				side := "buy"

				if step%3 == 0 {
					side = "sell"
					sellCount++
				} else {
					buyCount++
				}

				res := instrument.Step(trade(at, int64(step+1), side, 50000, 1))
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "hawkes")
				So(res.Label, ShouldEqual, "BTC/USD")
				So(res.At, ShouldEqual, at)
				So(res.From.Sub(origin).Abs(), ShouldBeLessThan, time.Microsecond)

				total := buyCount + sellCount
				So(metricValue(res, "event_count"), ShouldEqual, total)
				So(metricValue(res, "event_count:buy"), ShouldEqual, buyCount)
				So(metricValue(res, "event_count:sell"), ShouldEqual, sellCount)

				fracBuy := metricValue(res, "event_fraction:buy")
				fracSell := metricValue(res, "event_fraction:sell")
				So(fracBuy, ShouldAlmostEqual, buyCount/total, 1e-9)
				So(fracSell, ShouldAlmostEqual, sellCount/total, 1e-9)
				So(fracBuy+fracSell, ShouldAlmostEqual, 1.0, 1e-9)

				span := at.Sub(origin).Seconds()
				if span > 0 {
					So(metricValue(res, "arrival_rate:buy"), ShouldAlmostEqual, buyCount/span, 1e-4)
					So(metricValue(res, "arrival_rate:sell"), ShouldAlmostEqual, sellCount/span, 1e-4)
					So(metricValue(res, "arrival_rate"), ShouldAlmostEqual, (buyCount+sellCount)/span, 1e-4)
				}

				So(res.Maturity(), ShouldBeGreaterThanOrEqualTo, 0)
				So(res.Maturity(), ShouldBeLessThanOrEqualTo, 1.0)
			}
		})

		Convey("It drops an invalid trade side without publishing", func() {
			prior := data.NewMeasurement(
				1, "BTC/USD", "ingress", 100, 100,
				data.StringEntry{Key: "side", Value: "neutral"},
				data.StringEntry{Key: "channel", Value: "trade"},
			)
			prior.At = origin
			prior.From = origin
			prior = prior.Write(
				data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})

		Convey("It drops non-trade channel measurements without processing", func() {
			prior := data.NewMeasurement(
				1, "BTC/USD", "ingress", 101, 101,
				data.StringEntry{Key: "side", Value: "buy"},
				data.StringEntry{Key: "channel", Value: "book"},
			)
			prior.At = origin
			prior.From = origin
			prior = prior.Write(
				data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})
	})
}

func metricValue(measurement *data.Measurement, label string) float64 {
	got, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return got
}

