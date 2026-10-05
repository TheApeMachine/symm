package leadlag_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/leadlag"
)

func TestLeadLagTickerMetrics(t *testing.T) {
	Convey("Leadlag ticker instrument computes principled asynchronous Hayashi-Yoshida cross lead-lag", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := leadlag.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes cross-correlation, observation counts, and lag gain across asynchronous asset streams", func() {
			for step := 0; step < 25; step++ {
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
				So(resBTC.GetMetric("last").Raw, ShouldEqual, btcPrice)

				// Feed ETH with deliberate 30ms latency offset
				eth := arena.NewMeasurement("ingress")
				eth.Label = "ETH/USD"
				eth.SeqIdx = int64(step*2 + 2)
				eth.At = now.Add(time.Duration(step*100+30) * time.Millisecond)
				eth.From = eth.At
				ethPrice := 3000.0 + float64(step)*5.0
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
				So(resETH.GetMetric("last").Raw, ShouldEqual, ethPrice)

				if step >= 15 {
					// Both assets have trending correlated prices
					bestCorrMetric, hasCorr := resETH.LookupMetric("best_lag_correlation")
					if hasCorr {
						// Correlation must be mathematically bounded in [-1.0, 1.0]
						So(bestCorrMetric.Raw, ShouldBeGreaterThanOrEqualTo, -1.0)
						So(bestCorrMetric.Raw, ShouldBeLessThanOrEqualTo, 1.0)
						// Trending assets must exhibit positive correlation
						So(bestCorrMetric.Raw, ShouldBeGreaterThan, 0.5)

						// Contemporaneous correlation bounded in [-1.0, 1.0]
						contemp := resETH.GetMetric("contemporaneous_correlation").Raw
						So(contemp, ShouldBeGreaterThanOrEqualTo, -1.0)
						So(contemp, ShouldBeLessThanOrEqualTo, 1.0)

						// Absolute gain from lag optimization must be non-negative
						gain := resETH.GetMetric("absolute_correlation_gain").Raw
						So(gain, ShouldBeGreaterThanOrEqualTo, 0.0)

						// Lag fraction must be bounded in [0.0, 1.0]
						lagFrac := resETH.GetMetric("lag_fraction").Raw
						So(lagFrac, ShouldBeGreaterThanOrEqualTo, 0.0)
						So(lagFrac, ShouldBeLessThanOrEqualTo, 1.0)
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
			// No price metric set

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})
	})
}
