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
		small := normalizedTouchRatio(0.00015, 0.0005, false)
		large := normalizedTouchRatio(0.15, 0.5, false)

		Convey("It preserves the observed fraction", func() {
			So(*small, ShouldEqual, 0.00015/0.0005)
			So(*large, ShouldEqual, 0.15/0.5)
		})
	})

	Convey("Given the fraction boundaries", t, func() {
		So(*normalizedTouchRatio(0, 10, false), ShouldEqual, 0.0)
		So(*normalizedTouchRatio(10, 10, false), ShouldEqual, 1.0)
		So(normalizedTouchRatio(25, 10, false), ShouldBeNil)
	})

	Convey("Given quantities that compete with the previous resting quantity", t, func() {
		So(*normalizedTouchRatio(10, 10, true), ShouldEqual, 10.0/(10.0+10.0))
		So(*normalizedTouchRatio(30, 10, true), ShouldEqual, 30.0/(30.0+10.0))
	})

	Convey("Given no positive resting quantity to normalize against", t, func() {
		So(normalizedTouchRatio(0, 0, false), ShouldBeNil)
		So(normalizedTouchRatio(1, 0, false), ShouldBeNil)
		So(normalizedTouchRatio(1, -1, false), ShouldBeNil)
	})
}

func TestNormalizedTouchPrice(t *testing.T) {
	Convey("Given the same relative price move at different quote scales", t, func() {
		base := normalizedTouchPrice(102, 100)
		scaled := normalizedTouchPrice(204, 200)
		reversed := normalizedTouchPrice(100, 102)

		So(*base, ShouldEqual, math.Log(102.0/100.0))
		So(*scaled, ShouldEqual, *base)
		So(*reversed, ShouldEqual, math.Log(100.0/102.0))
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
	Convey("Given the first observed touch without a prior bracket", t, func() {
		at := time.Unix(1_700_004_450, 0).UTC()
		touch := touchSnapshot{
			asOf: at,
			bid:  touchObservation{price: 100, quantity: 10},
			ask:  touchObservation{price: 101, quantity: 20},
		}
		measurement := toxicityMeasurement("BTC/USD", touch, touch, nil)

		Convey("It should emit only the raw prior needed by the next observation", func() {
			So(measurement.Metrics, ShouldHaveLength, 4)
			So(measurement.ObservedFrom.IsZero(), ShouldBeTrue)
			So(measurement.Horizon, ShouldEqual, time.Duration(0))
			So(measurement.Sample(types.MetricBestPrice, types.SideBuy),
				ShouldResemble, types.MetricSample{
					Raw: 100, Unit: types.UnitQuoteCurrency,
				})
			So(measurement.Sample(types.MetricTouchQuantity, types.SideSell),
				ShouldResemble, types.MetricSample{
					Raw: 20, Unit: types.UnitBaseCurrency,
				})
		})
	})

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
			execution := math.Sqrt((math.Pow(0.2, 2) + math.Pow(0.25, 2)) / 2)
			retreat := math.Sqrt((math.Pow(0.8, 2) + math.Pow(0, 2)) / 2)
			cancellation := math.Sqrt((math.Pow(0, 2) + math.Pow(0.15, 2)) / 2)
			noiseFloor := math.Sqrt(
				math.Pow(execution, 2) + math.Pow(cancellation, 2),
			)
			expected := map[string]float64{
				types.MetricKey(types.MetricSNR, types.SideNone):                (retreat - noiseFloor) / retreat,
				types.MetricKey(types.MetricTradeVolume, types.SideNone):        7.0 / 37.0,
				types.MetricKey(types.MetricFillVolume, types.SideBuy):          2.0 / 10.0,
				types.MetricKey(types.MetricFillVolume, types.SideSell):         5.0 / 20.0,
				types.MetricKey(types.MetricBestPrice, types.SideBuy):           math.Log(99.0 / 100.0),
				types.MetricKey(types.MetricBestPrice, types.SideSell):          0,
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy):       8.0 / 18.0,
				types.MetricKey(types.MetricTouchQuantity, types.SideSell):      12.0 / 32.0,
				types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy):  8.0 / 10.0,
				types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): 0,
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy):   0,
				types.MetricKey(types.MetricCancelledQuantity, types.SideSell):  3.0 / 20.0,
			}

			for metric, normalized := range expected {
				sample := measurement.Metrics[metric]
				So(sample.Normalized, ShouldNotBeNil)
				So(*sample.Normalized, ShouldEqual, normalized)
			}
			So(measurement.Metrics, ShouldHaveLength, 12)
			So(measurement.ObservedFrom, ShouldResemble, previous.asOf)
			So(measurement.At, ShouldResemble, current.asOf)
			So(measurement.Horizon, ShouldEqual, time.Second)
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
				So(*scaled.Metrics[metric].Normalized, ShouldEqual, *sample.Normalized)
			}
		})
	})

	Convey("Given more tape volume than could fill the previous touch", t, func() {
		from := time.Unix(1_700_004_600, 0).UTC()
		previous := touchSnapshot{
			asOf: from,
			bid:  touchObservation{price: 100, quantity: 3},
			ask:  touchObservation{price: 101, quantity: 5},
		}
		current := touchSnapshot{
			asOf: from.Add(time.Second),
			bid:  touchObservation{price: 99, quantity: 2},
			ask:  touchObservation{price: 101, quantity: 1},
		}
		measurement := toxicityMeasurement(
			"BTC/USD",
			previous,
			current,
			[]kraken.TradeData{
				toxicityTrade(3, "sell", 100, 11, from.Add(time.Second)),
				toxicityTrade(4, "buy", 101, 13, from.Add(time.Second)),
			},
		)

		Convey("It caps fills at physically displayed quantity and cannot invent cancellation", func() {
			So(measurement.Sample(types.MetricFillVolume, types.SideBuy).Raw,
				ShouldEqual, 3.0)
			So(measurement.Sample(types.MetricFillVolume, types.SideSell).Raw,
				ShouldEqual, 5.0)
			So(measurement.Sample(types.MetricRetreatingQuantity, types.SideBuy).Raw,
				ShouldEqual, 0.0)
			So(measurement.Sample(types.MetricCancelledQuantity, types.SideSell).Raw,
				ShouldEqual, 0.0)
		})
	})
}
