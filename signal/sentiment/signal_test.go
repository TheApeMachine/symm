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
		1, label, "spot:trade", seq, seq,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: "buy"},
	)
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", 1, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		return entry.Metric.Raw, true
	}

	return 0, false
}

func TestSentimentSignalMetrics(t *testing.T) {
	Convey("Given a READY sentiment signal", t, func() {
		instrument := sentiment.NewSignal(context.Background(), data.NewArenaOwner("sentiment", 4096))
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD", "XRP/USD"}

		Convey("It measures advance/decline breadth, unchanged counts, and the median change", func() {
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

					if step == 0 {
						count, held := metric(res, "valid_member_count")
						So(held, ShouldBeTrue)
						So(count, ShouldEqual, 0)

						b, held := metric(res, "breadth")
						So(held, ShouldBeTrue)
						So(b, ShouldEqual, 0)
						m, held := metric(res, "median_return")
						So(held, ShouldBeTrue)
						So(m, ShouldEqual, 0)
					}

					if step > 1 && idx == len(symbols)-1 {
						expected := map[string]float64{
							"valid_member_count": 5,
							"advance_count":      2,
							"decline_count":      2,
							"unchanged_count":    1,
							"advance_fraction":   2.0 / 5.0,
							"decline_fraction":   2.0 / 5.0,
							"unchanged_fraction": 1.0 / 5.0,
							"breadth":            0,
							"median_return":      0,
						}

						for label, want := range expected {
							got, held := metric(res, label)
							So(held, ShouldBeTrue)
							So(got, ShouldAlmostEqual, want, 1e-9)
						}

						for _, label := range []string{
							"breadth_baseline", "breadth_divergence",
							"median_return_baseline", "median_return_divergence",
							"median_return_velocity", "breadth_velocity",
						} {
							_, held := metric(res, label)
							So(held, ShouldBeTrue)
						}

						median, _ := metric(res, "median_return")
						baseline, _ := metric(res, "median_return_baseline")
						divergence, _ := metric(res, "median_return_divergence")
						So(divergence, ShouldAlmostEqual, median-baseline, 1e-12)
					}
				}
			}
		})

		Convey("A market-wide advance produces full advance breadth and a positive median change", func() {
			for step := range 4 {
				for idx, symbol := range symbols {
					seq := int64(step*len(symbols) + idx + 100)
					at := origin.Add(time.Duration(seq) * 100 * time.Millisecond)
					price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.02)

					res := instrument.Step(trade(symbol, at, seq, price))
					So(res, ShouldNotBeNil)
					So(instrument.Error(), ShouldBeNil)

					if step > 1 && idx == len(symbols)-1 {
						expected := map[string]float64{
							"advance_count":    5,
							"decline_count":    0,
							"unchanged_count":  0,
							"advance_fraction": 1,
							"decline_fraction": 0,
							"breadth":          1,
						}

						for label, want := range expected {
							got, held := metric(res, label)
							So(held, ShouldBeTrue)
							So(got, ShouldAlmostEqual, want, 1e-9)
						}

						median, held := metric(res, "median_return")
						So(held, ShouldBeTrue)
						So(median, ShouldBeGreaterThan, 0)
					}
				}
			}
		})

		Convey("It yields no measurement for a trade without a positive price", func() {
			So(instrument.Step(trade("BTC/USD", origin, 1, 0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("It drops events before READY", func() {
			cold := sentiment.NewSignal(context.Background(), data.NewArenaOwner("sentiment", 16))
			So(cold.Step(trade("BTC/USD", origin, 1, 100)), ShouldBeNil)
		})
	})
}
