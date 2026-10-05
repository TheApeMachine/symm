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

func TestSentimentTickerMetrics(t *testing.T) {
	Convey("Sentiment ticker instrument measures cross-sectional breadth, returns, and dispersion", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := sentiment.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD", "XRP/USD"}

		Convey("Measures exact advance/decline breadth, unchanged counts, and median returns across cohort", func() {
			for step := 0; step < 6; step++ {
				for idx, symbol := range symbols {
					prior := arena.NewMeasurement("ingress")
					prior.Label = symbol
					prior.SeqIdx = int64(step*len(symbols) + idx + 1)
					prior.At = now.Add(time.Duration(step*len(symbols)+idx) * 100 * time.Millisecond)
					prior.From = prior.At

					// 2 declining (idx 0, 1), 1 flat (idx 2), 2 advancing (idx 3, 4)
					price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.01*float64(idx-2))
					prior.SetMetric("last", data.NewMetric(
						"last",
						data.UnitPrice,
						data.TimescaleInstantaneous,
						price,
						1.0,
					).Write(price))

					res := instrument.Step(prior)
					So(res, ShouldNotBeNil)
					So(res.Err, ShouldBeNil)

					// After initial step establishes price baselines, subsequent steps calculate returns
					if step > 1 && idx == len(symbols)-1 {
						validMembers := res.GetMetric("valid_member_count").Raw
						So(validMembers, ShouldEqual, 5)

						// 2 advancing, 2 declining, 1 unchanged
						So(res.GetMetric("advance_count").Raw, ShouldEqual, 2)
						So(res.GetMetric("decline_count").Raw, ShouldEqual, 2)
						So(res.GetMetric("unchanged_count").Raw, ShouldEqual, 1)

						So(res.GetMetric("advance_fraction").Raw, ShouldAlmostEqual, 2.0/5.0, 1e-9)
						So(res.GetMetric("decline_fraction").Raw, ShouldAlmostEqual, 2.0/5.0, 1e-9)
						So(res.GetMetric("unchanged_fraction").Raw, ShouldAlmostEqual, 1.0/5.0, 1e-9)

						// Symmetric distribution has breadth = 0 and median return = 0
						So(res.GetMetric("breadth").Raw, ShouldAlmostEqual, 0.0, 1e-9)
						So(res.GetMetric("median_return").Raw, ShouldAlmostEqual, 0.0, 1e-6)

						// Dispersion is positive
						So(res.GetMetric("return_mad").Raw, ShouldBeGreaterThan, 0.0)
						So(res.GetMetric("median_absolute_return").Raw, ShouldBeGreaterThan, 0.0)
					}
				}
			}
		})

		Convey("Market-wide bull run produces 100% advance breadth and positive median return", func() {
			bullInstrument := sentiment.NewSignal(ctx, arena)
			bullInstrument.Transition(nmruntime.READY)

			for step := 0; step < 4; step++ {
				for idx, symbol := range symbols {
					prior := arena.NewMeasurement("ingress")
					prior.Label = symbol
					prior.SeqIdx = int64(step*len(symbols) + idx + 100)
					prior.At = now.Add(time.Duration(step*len(symbols)+idx) * 100 * time.Millisecond)
					prior.From = prior.At

					// All symbols advancing by +2% each step
					price := 100.0 * float64(idx+1) * (1.0 + float64(step)*0.02)
					prior.SetMetric("last", data.NewMetric(
						"last",
						data.UnitPrice,
						data.TimescaleInstantaneous,
						price,
						1.0,
					).Write(price))

					res := bullInstrument.Step(prior)
					So(res, ShouldNotBeNil)

					if step > 1 && idx == len(symbols)-1 {
						So(res.GetMetric("advance_count").Raw, ShouldEqual, 5)
						So(res.GetMetric("decline_count").Raw, ShouldEqual, 0)
						So(res.GetMetric("unchanged_count").Raw, ShouldEqual, 0)
						So(res.GetMetric("advance_fraction").Raw, ShouldAlmostEqual, 1.0, 1e-9)
						So(res.GetMetric("decline_fraction").Raw, ShouldAlmostEqual, 0.0, 1e-9)
						So(res.GetMetric("breadth").Raw, ShouldAlmostEqual, 1.0, 1e-9)
						So(res.GetMetric("median_return").Raw, ShouldBeGreaterThan, 0.0)
					}
				}
			}
		})
	})
}
