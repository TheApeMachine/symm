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
		1, "BTC/USD", "spot:trade", at, at,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: side},
	)
	prior.SeqIdx = seq

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

		return entry.Metric.Raw(), true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func TestHawkesTradeMetrics(t *testing.T) {
	Convey("Given a READY Hawkes arrival-dynamics signal", t, func() {
		instrument := hawkes.NewSignal(context.Background())
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("It computes exact arrival rates for buy and sell", func() {
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
				So(data.Pull(res.Meta("side")).Value, ShouldEqual, side)
				So(res.At, ShouldEqual, at)
				So(res.From.Sub(origin).Abs(), ShouldBeLessThan, time.Microsecond)

				span := at.Sub(origin).Seconds()
				if span > 0 {
					So(metricValue(res, "arrival_rate:buy"), ShouldAlmostEqual, buyCount/span, 1e-4)
					So(metricValue(res, "arrival_rate:sell"), ShouldAlmostEqual, sellCount/span, 1e-4)
				}
			}
		})

		Convey("It drops an invalid trade side without publishing", func() {
			prior := data.NewMeasurement(
				1, "BTC/USD", "spot:trade", origin, origin,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: "neutral"},
			)
			prior = prior.Write(
				data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})

		Convey("A touch frame is ignored without error", func() {
			touch := data.NewMeasurement(1, "BTC/USD", "spot:touch", origin, origin)
			res := instrument.Step(touch)
			So(res, ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It treats a non-trade frame as a wiring error", func() {
			prior := data.NewMeasurement(
				1, "BTC/USD", "spot:level3", origin, origin,
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			prior = prior.Write(
				data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
			So(instrument.Error(), ShouldNotBeNil)
		})
	})
}
