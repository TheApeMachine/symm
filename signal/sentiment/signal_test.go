package sentiment_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/sentiment"
)

func trade(label string, at time.Time, seq int64, price float64) *data.Measurement {
	prior := data.NewMeasurement(
		1, label, "spot:trade", at, at,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: "buy"},
	)

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw(), true
	}

	return 0, false
}

func TestSentimentSignalMetrics(t *testing.T) {
	Convey("Given a READY sentiment signal", t, func() {
		instrument := sentiment.NewSignal(context.Background())
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD", "XRP/USD"}

		Convey("It measures breadth, breadth_velocity, and peer direction counts", func() {
			for step := range 6 {
				for idx, symbol := range symbols {
					seq := int64(step*len(symbols) + idx + 1)
					at := origin.Add(time.Duration(seq) * 100 * time.Millisecond)

					// 2 declining (idx 0, 1), 1 flat (idx 2), 2 advancing (idx 3, 4).
					price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.01*float64(idx-2))

					res := instrument.Step(trade(symbol, at, seq, price))
					So(res, ShouldNotBeNil)
					So(instrument.Error(), ShouldBeNil)
					So(res.Source, ShouldEqual, "sentiment")
					So(res.Label, ShouldEqual, symbol)

					if step > 1 && idx == len(symbols)-1 {
						breadth, held := metric(res, "breadth")
						So(held, ShouldBeTrue)
						So(breadth, ShouldAlmostEqual, 0.0, 1e-9)

						_, held = metric(res, "same_direction_peer_count")
						So(held, ShouldBeTrue)

						_, held = metric(res, "opposite_direction_peer_count")
						So(held, ShouldBeTrue)

						_, held = metric(res, "breadth_velocity")
						So(held, ShouldBeTrue)
					}
				}
			}
		})

		Convey("A market-wide advance produces breadth = 1.0", func() {
			for step := range 4 {
				for idx, symbol := range symbols {
					seq := int64(step*len(symbols) + idx + 100)
					at := origin.Add(time.Duration(seq) * 100 * time.Millisecond)
					price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.02)

					res := instrument.Step(trade(symbol, at, seq, price))
					So(res, ShouldNotBeNil)
					So(instrument.Error(), ShouldBeNil)

					if step > 1 && idx == len(symbols)-1 {
						breadth, held := metric(res, "breadth")
						So(held, ShouldBeTrue)
						So(breadth, ShouldAlmostEqual, 1.0, 1e-9)

						sameCount, held := metric(res, "same_direction_peer_count")
						So(held, ShouldBeTrue)
						So(sameCount, ShouldEqual, 4)

						oppCount, held := metric(res, "opposite_direction_peer_count")
						So(held, ShouldBeTrue)
						So(oppCount, ShouldEqual, 0)
					}
				}
			}
		})

		Convey("It yields no measurement for a trade without a positive price", func() {
			So(instrument.Step(trade("BTC/USD", origin, 1, 0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It drops events before READY", func() {
			cold := sentiment.NewSignal(context.Background())
			So(cold.Step(trade("BTC/USD", origin, 1, 100)), ShouldBeNil)
		})
	})
}
