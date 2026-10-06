package correlation_test

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
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

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Correlation signal measures principled asynchronous price-path co-movements and cohort metrics", t, func() {
		ctx := context.Background()
		instrument := correlation.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes bounded signed and absolute correlations across co-trending assets", func() {
			measuredPairs := 0

			// One latent log-price random walk on a 10ms grid. BTC samples it on
			// the 100ms grid; ETH samples it, plus small idiosyncratic noise,
			// 20ms later: asynchronous co-movement without a shared clock.
			random := rand.New(rand.NewSource(7))
			latent := make([]float64, 25*10+3)

			for index := 1; index < len(latent); index++ {
				latent[index] = latent[index-1] + random.NormFloat64()*1e-3
			}

			for step := range 25 {
				btcPrice := 50000.0 * math.Exp(latent[step*10])
				resBTC := instrument.Step(trade(
					"BTC/USD", now.Add(time.Duration(step*100)*time.Millisecond), int64(step*2+1), btcPrice,
				))
				So(resBTC, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				lastBTC, held := metric(resBTC, "last_price")
				So(held, ShouldBeTrue)
				So(lastBTC, ShouldEqual, btcPrice)

				// ETH follows BTC's moves with a 20ms asynchronous offset.
				ethPrice := 3000.0 * math.Exp(latent[step*10+2]+random.NormFloat64()*1e-4)
				resETH := instrument.Step(trade(
					"ETH/USD", now.Add(time.Duration(step*100+20)*time.Millisecond), int64(step*2+2), ethPrice,
				))
				So(resETH, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				lastETH, held := metric(resETH, "last_price")
				So(held, ShouldBeTrue)
				So(lastETH, ShouldEqual, ethPrice)

				if step >= 15 {
					count, held := metric(resETH, "observation_count")
					So(held, ShouldBeTrue)
					So(count, ShouldBeGreaterThanOrEqualTo, 15)

					if signed, ok := metric(resETH, "signed_correlation@BTC/USD"); ok {
						measuredPairs++
						So(signed, ShouldBeGreaterThanOrEqualTo, -1.0)
						So(signed, ShouldBeLessThanOrEqualTo, 1.0)
						So(signed, ShouldBeGreaterThan, 0.5)

						absolute, held := metric(resETH, "absolute_correlation@BTC/USD")
						So(held, ShouldBeTrue)
						So(absolute, ShouldBeBetweenOrEqual, 0.0, 1.0)
					}

					if cohort, ok := metric(resETH, "cohort_signed_correlation"); ok {
						So(cohort, ShouldBeBetweenOrEqual, -1.0, 1.0)
						So(cohort, ShouldBeGreaterThan, 0.5)

						cohortAbs, held := metric(resETH, "cohort_absolute_correlation")
						So(held, ShouldBeTrue)
						So(cohortAbs, ShouldBeBetweenOrEqual, 0.0, 1.0)
					}

					// The reference symbol measures the focal one from its own side.
					if signed, ok := metric(resBTC, "signed_correlation@ETH/USD"); ok {
						So(signed, ShouldBeBetweenOrEqual, -1.0, 1.0)
					}
				}
			}

			So(measuredPairs, ShouldBeGreaterThan, 0)
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
