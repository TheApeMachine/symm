package correlation_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
)

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Correlation signal measures principled asynchronous price-path co-movements and cohort metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner("test", 4096)

		instrument := correlation.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes bounded signed and absolute correlations across co-trending assets", func() {
			for step := range 25 {
				// Feed BTC
				btc := arena.NewMeasurement("ingress")
				btc.Label = "BTC/USD"
				btc.SeqIdx = int64(step*2 + 1)
				btc.At = now.Add(time.Duration(step*100) * time.Millisecond)
				btc.From = btc.At
				btcPrice := 50000.0 + float64(step)*10.0
				btc.SetMetric("last", data.NewMetric(
					"last",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					btcPrice,
					1.0,
				).Write(btcPrice))

				resBTC := instrument.Step(btc)
				So(resBTC, ShouldNotBeNil)
				So(resBTC.Err, ShouldBeNil)
				So(resBTC.GetMetric("last_price").Raw, ShouldEqual, btcPrice)

				// Feed ETH with 20ms asynchronous offset
				eth := arena.NewMeasurement("ingress")
				eth.Label = "ETH/USD"
				eth.SeqIdx = int64(step*2 + 2)
				eth.At = now.Add(time.Duration(step*100+20) * time.Millisecond)
				eth.From = eth.At
				ethPrice := 3000.0 + float64(step)*2.0
				eth.SetMetric("last", data.NewMetric(
					"last",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					ethPrice,
					1.0,
				).Write(ethPrice))

				resETH := instrument.Step(eth)
				So(resETH, ShouldNotBeNil)
				So(resETH.Err, ShouldBeNil)
				So(resETH.GetMetric("last_price").Raw, ShouldEqual, ethPrice)

				if step >= 15 {
					// Check observation counts
					So(resETH.GetMetric("observation_count").Raw, ShouldBeGreaterThanOrEqualTo, 15)

					if signedCorr, ok := resETH.LookupMetric("signed_correlation"); ok {
						// Mathematical bounds on correlation
						So(signedCorr.Raw, ShouldBeGreaterThanOrEqualTo, -1.0)
						So(signedCorr.Raw, ShouldBeLessThanOrEqualTo, 1.0)
						// Trending together must produce positive correlation
						So(signedCorr.Raw, ShouldBeGreaterThan, 0.5)

						absCorr := resETH.GetMetric("absolute_correlation").Raw
						So(absCorr, ShouldBeGreaterThanOrEqualTo, 0.0)
						So(absCorr, ShouldBeLessThanOrEqualTo, 1.0)
					}

					if cohortSigned, ok := resETH.LookupMetric("cohort_signed_correlation"); ok {
						So(cohortSigned.Raw, ShouldBeGreaterThanOrEqualTo, -1.0)
						So(cohortSigned.Raw, ShouldBeLessThanOrEqualTo, 1.0)
						So(cohortSigned.Raw, ShouldBeGreaterThan, 0.5)

						cohortAbs := resETH.GetMetric("cohort_absolute_correlation").Raw
						So(cohortAbs, ShouldBeGreaterThanOrEqualTo, 0.0)
						So(cohortAbs, ShouldBeLessThanOrEqualTo, 1.0)
					}
				}
			}
		})

		Convey("Rejects non-positive or missing prices cleanly", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 999
			prior.At = now
			prior.From = now
			// Missing price

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})
	})
}
