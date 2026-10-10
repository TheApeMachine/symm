package leadlag_test

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/tests/market"
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
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw, true
	}

	return 0, false
}

/*
summaryKeys are the cross-peer summaries leadlag emits once a peer is defined.
*/
var summaryKeys = []string{
	"defined_peer_count",
	"best_lag_seconds_median",
	"best_lag_seconds_mad",
	"led_peer_share",
	"covariance_score_gain_mean",
	"covariance_score_gain_median",
}

func TestLeadLagSignalMetrics(t *testing.T) {
	Convey("Leadlag instrument computes principled asynchronous Hayashi-Yoshida cross lead-lag", t, func() {
		ctx := context.Background()

		instrument := leadlag.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Recovers a positive lag when the reference path leads the measured path", func() {
			// One latent log-price random walk on a 10ms grid. BTC samples it on
			// the 100ms grid; ETH samples it 30ms later, but sees the latent path
			// as it stood 200ms earlier: BTC leads ETH by two search steps.
			random := rand.New(rand.NewSource(11))
			latent := make([]float64, 40*10+21)

			for index := 1; index < len(latent); index++ {
				latent[index] = latent[index-1] + random.NormFloat64()*1e-3
			}

			measured := 0

			for step := range 40 {
				btcPrice := 50000.0 * math.Exp(latent[step*10+20])
				resBTC := instrument.Step(trade(
					"BTC/USD", now.Add(time.Duration(step*100)*time.Millisecond), int64(step*2+1), btcPrice,
				))
				So(resBTC, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				lastBTC, held := metric(resBTC, "last")
				So(held, ShouldBeTrue)
				So(lastBTC, ShouldEqual, btcPrice)

				ethPrice := 3000.0 * math.Exp(latent[step*10+3])
				resETH := instrument.Step(trade(
					"ETH/USD", now.Add(time.Duration(step*100+30)*time.Millisecond), int64(step*2+2), ethPrice,
				))
				So(resETH, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				lastETH, held := metric(resETH, "last")
				So(held, ShouldBeTrue)
				So(lastETH, ShouldEqual, ethPrice)

				for _, res := range []*data.Measurement{resBTC, resETH} {
					for entry := range res.Read() {
						So(entry.Key, ShouldNotContainSubstring, "@")
					}
				}

				if step == 0 {
					// BTC arrives before any peer path exists: no defined peer,
					// so no summary is emitted at all.
					for _, key := range summaryKeys {
						_, held := metric(resBTC, key)
						So(held, ShouldBeFalse)
					}
				}

				if step < 30 {
					continue
				}

				peers, ok := metric(resETH, "defined_peer_count")
				if !ok {
					continue
				}

				measured++
				So(peers, ShouldEqual, 1)

				lag, held := metric(resETH, "best_lag_seconds_median")
				So(held, ShouldBeTrue)
				So(lag, ShouldBeGreaterThan, 0)

				spread, held := metric(resETH, "best_lag_seconds_mad")
				So(held, ShouldBeTrue)
				So(spread, ShouldEqual, 0)

				gain, held := metric(resETH, "covariance_score_gain_median")
				So(held, ShouldBeTrue)
				So(gain, ShouldBeGreaterThan, 0)

				share, held := metric(resETH, "led_peer_share")
				So(held, ShouldBeTrue)
				So(share, ShouldEqual, 0)

				leaderLag, held := metric(resBTC, "best_lag_seconds_median")
				So(held, ShouldBeTrue)
				So(leaderLag, ShouldBeLessThan, 0)
			}

			So(measured, ShouldBeGreaterThan, 0)
		})

		Convey("Rejects a non-positive trade price without error", func() {
			So(instrument.Step(trade("BTC/USD", now, 998, 0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("Treats a trade frame without price as an error", func() {
			prior := data.NewMeasurement(1, "BTC/USD", "spot:trade", 999, 999)
			prior.At = now
			prior.From = now

			So(instrument.Step(prior.Write()), ShouldBeNil)
			So(instrument.Error(), ShouldNotBeNil)
		})
	})
}

func TestLeadLagCapturedBoundaryTape(t *testing.T) {
	Convey("Given the captured CRV/DOT trade tape whose uneven spacing put the best lag on the profile boundary", t, func() {
		instrument := leadlag.NewSignal(context.Background())
		instrument.Transition(nmruntime.READY)

		Convey("Every spot:trade frame steps without a panic or an error", func() {
			for _, frame := range market.LeadLagTape() {
				So(frame.Source, ShouldEqual, "spot:trade")
				So(func() { instrument.Step(frame) }, ShouldNotPanic)
				So(instrument.Error(), ShouldBeNil)
			}
		})
	})
}
