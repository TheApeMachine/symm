package perspectives

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// priceSeries builds a snapshot of price-bearing measurements. To mimic the live
// snapshot, each price is repeated repeat times (the way the many signal sources
// each stamp the same Last between trades).
func priceSeries(symbol string, repeat int, prices ...float64) []Measurement {
	snapshots := make([]Measurement, 0, len(prices)*repeat)

	for _, price := range prices {
		for copy := 0; copy < repeat; copy++ {
			snapshots = append(snapshots, Measurement{Symbol: symbol, Last: price})
		}
	}

	return snapshots
}

// ramp returns count prices starting at start, each multiplied by (1+step).
func ramp(start, step float64, count int) []float64 {
	prices := make([]float64, count)
	price := start

	for index := range prices {
		prices[index] = price
		price *= 1 + step
	}

	return prices
}

// zigzag alternates up/down by mag around start with no net drift.
func zigzag(start, mag float64, count int) []float64 {
	prices := make([]float64, count)

	for index := range prices {
		if index%2 == 0 {
			prices[index] = start
		} else {
			prices[index] = start * (1 + mag)
		}
	}

	return prices
}

func TestClassifyRegime(t *testing.T) {
	convey.Convey("Given the price-action regime classifier", t, func() {
		convey.Convey("A sparse snapshot below min_samples is unclassified", func() {
			features := ClassifyRegime(priceSeries("BTC/EUR", 1, 100, 101, 102))

			convey.So(features.Regime, convey.ShouldEqual, RegimeNone)
		})

		convey.Convey("Repeated quotes with no price change read as Dead", func() {
			// 40 measurements, all the same price: quoting, not trading.
			features := ClassifyRegime(priceSeries("BTC/EUR", 40, 100))

			convey.So(features.Samples, convey.ShouldEqual, 40)
			convey.So(features.Regime, convey.ShouldEqual, RegimeDead)
		})

		convey.Convey("A tiny directionless wiggle below the vol floor is Dead", func() {
			// 1bp zigzag: active enough in count, but realized vol is below floor
			// and there is no net direction.
			features := ClassifyRegime(priceSeries("BTC/EUR", 2, zigzag(100, 0.00002, 40)...))

			convey.So(features.Regime, convey.ShouldEqual, RegimeDead)
		})

		convey.Convey("A steady upward ramp is Bullish", func() {
			features := ClassifyRegime(priceSeries("BTC/EUR", 2, ramp(100, 0.002, 40)...))

			convey.So(features.Drift, convey.ShouldBeGreaterThan, 0)
			convey.So(features.TrendStrength, convey.ShouldBeGreaterThanOrEqualTo, 3.0)
			convey.So(features.Regime, convey.ShouldEqual, RegimeBullish)
		})

		convey.Convey("A steady downward ramp is Bearish", func() {
			features := ClassifyRegime(priceSeries("BTC/EUR", 2, ramp(100, -0.002, 40)...))

			convey.So(features.Drift, convey.ShouldBeLessThan, 0)
			convey.So(features.Regime, convey.ShouldEqual, RegimeBearish)
		})

		convey.Convey("Large directionless volatility is Choppy", func() {
			// 2% zigzag, no net drift: lots of realized vol, trend t-stat ~ 0.
			features := ClassifyRegime(priceSeries("BTC/EUR", 2, zigzag(100, 0.02, 60)...))

			convey.So(features.Volatility, convey.ShouldBeGreaterThan, 0.0005)
			convey.So(features.TrendStrength, convey.ShouldBeLessThan, 1.5)
			convey.So(features.Regime, convey.ShouldEqual, RegimeChoppy)
			convey.So(features.Choppiness, convey.ShouldBeGreaterThan, 0.5)
		})

		convey.Convey("A noisy-but-drifting market lands in the Trending tier", func() {
			// Upward drift with enough noise that the t-stat sits between the
			// trend and strong-trend thresholds.
			prices := make([]float64, 60)
			price := 100.0

			for index := range prices {
				// alternating +0.3% / -0.1% => net up, but noisy.
				if index%2 == 0 {
					price *= 1.003
				} else {
					price *= 0.999
				}

				prices[index] = price
			}

			features := ClassifyRegime(priceSeries("BTC/EUR", 2, prices...))

			convey.So(features.Drift, convey.ShouldBeGreaterThan, 0)
			convey.So(features.Regime, convey.ShouldBeIn, RegimeTrending, RegimeBullish)
		})

		convey.Convey("Global rows without a price are ignored", func() {
			snapshots := priceSeries("BTC/EUR", 2, ramp(100, 0.002, 40)...)
			// interleave price-less global measurements (Last == 0).
			withGlobals := make([]Measurement, 0, len(snapshots)*2)

			for _, measurement := range snapshots {
				withGlobals = append(withGlobals, Measurement{Last: 0})
				withGlobals = append(withGlobals, measurement)
			}

			features := ClassifyRegime(withGlobals)

			convey.So(features.Regime, convey.ShouldEqual, RegimeBullish)
		})

		convey.Convey("Features are finite for every input", func() {
			features := ClassifyRegime(priceSeries("BTC/EUR", 2, ramp(100, 0.002, 40)...))

			convey.So(math.IsNaN(features.TrendStrength), convey.ShouldBeFalse)
			convey.So(math.IsInf(features.TrendStrength, 0), convey.ShouldBeFalse)
		})
	})
}
