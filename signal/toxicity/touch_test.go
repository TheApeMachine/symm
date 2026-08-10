package toxicity

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestNormalizedTouchRatio(t *testing.T) {
	Convey("Given equivalent fractions at different quantity scales", t, func() {
		small := normalizedTouchRatio(0.015, 0.0005)
		large := normalizedTouchRatio(15, 0.5)

		Convey("It preserves the ratio instead of pretending normalization is a probability", func() {
			So(*small, ShouldAlmostEqual, 30, 1e-12)
			So(*large, ShouldAlmostEqual, 30, 1e-12)
		})
	})

	Convey("Given the fraction boundaries", t, func() {
		So(*normalizedTouchRatio(0, 10), ShouldEqual, 0.0)
		So(*normalizedTouchRatio(10, 10), ShouldEqual, 1.0)
		So(*normalizedTouchRatio(25, 10), ShouldEqual, 2.5)
	})

	Convey("Given no positive resting quantity to normalize against", t, func() {
		So(normalizedTouchRatio(0, 0), ShouldBeNil)
		So(normalizedTouchRatio(1, 0), ShouldBeNil)
		So(normalizedTouchRatio(1, -1), ShouldBeNil)
	})
}

func TestNormalizedTouchPrice(t *testing.T) {
	Convey("Given the same relative price move at different quote scales", t, func() {
		base := normalizedTouchPrice(102, 100)
		scaled := normalizedTouchPrice(204, 200)
		reversed := normalizedTouchPrice(100, 102)

		So(*base, ShouldAlmostEqual, math.Log(1.02), 1e-12)
		So(*scaled, ShouldAlmostEqual, *base, 1e-12)
		So(*reversed, ShouldAlmostEqual, -*base, 1e-12)
	})

	Convey("Given an unchanged valid touch price", t, func() {
		So(*normalizedTouchPrice(100, 100), ShouldEqual, 0.0)
	})

	Convey("Given a non-positive price", t, func() {
		So(normalizedTouchPrice(0, 100), ShouldBeNil)
		So(normalizedTouchPrice(100, 0), ShouldBeNil)
	})
}

func TestToxicityMeasurement(t *testing.T) {
	Convey("Given fills, a retreat, and an unexplained cancellation", t, func() {
		from := time.Unix(1_700_004_500, 0).UTC()
		previous := touchSnapshot{
			asOf: from,
			bid:  touchObservation{price: 100, quantity: 10},
			ask:  touchObservation{price: 101, quantity: 20},
		}
		current := touchSnapshot{
			asOf: from.Add(time.Second),
			bid:  touchObservation{price: 99, quantity: 8},
			ask:  touchObservation{price: 101, quantity: 12},
		}
		trades := []kraken.TradeData{
			toxicityTrade(1, "sell", 100, 2, from.Add(time.Second)),
			toxicityTrade(2, "buy", 101, 5, from.Add(time.Second)),
		}
		measurement := toxicityMeasurement("BTC/USD", previous, current, trades)

		Convey("It reports every ratio against the quantity that could cause it", func() {
			expected := map[string]float64{
				types.MetricKey(types.MetricTradeVolume, types.SideNone):        7.0 / 30.0,
				types.MetricKey(types.MetricFillVolume, types.SideBuy):          2.0 / 10.0,
				types.MetricKey(types.MetricFillVolume, types.SideSell):         5.0 / 20.0,
				types.MetricKey(types.MetricBestPrice, types.SideBuy):           math.Log(99.0 / 100.0),
				types.MetricKey(types.MetricBestPrice, types.SideSell):          0,
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy):       8.0 / 10.0,
				types.MetricKey(types.MetricTouchQuantity, types.SideSell):      12.0 / 20.0,
				types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy):  8.0 / 10.0,
				types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): 0,
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy):   0,
				types.MetricKey(types.MetricCancelledQuantity, types.SideSell):  3.0 / 20.0,
			}

			for metric, normalized := range expected {
				sample := measurement.Metrics[metric]
				So(sample.Normalized, ShouldNotBeNil)
				So(*sample.Normalized, ShouldAlmostEqual, normalized, 1e-12)
			}
		})

		Convey("It preserves every normalized value when base quantities are rescaled", func() {
			quantityScale := 100.0
			scaledPrevious := previous
			scaledPrevious.bid.quantity *= quantityScale
			scaledPrevious.ask.quantity *= quantityScale
			scaledCurrent := current
			scaledCurrent.bid.quantity *= quantityScale
			scaledCurrent.ask.quantity *= quantityScale
			scaledTrades := []kraken.TradeData{
				toxicityTrade(1, "sell", 100, 2*quantityScale, from.Add(time.Second)),
				toxicityTrade(2, "buy", 101, 5*quantityScale, from.Add(time.Second)),
			}
			scaled := toxicityMeasurement(
				"BTC/USD",
				scaledPrevious,
				scaledCurrent,
				scaledTrades,
			)

			for metric, sample := range measurement.Metrics {
				So(*scaled.Metrics[metric].Normalized,
					ShouldAlmostEqual, *sample.Normalized, 1e-12)
			}
		})
	})
}
