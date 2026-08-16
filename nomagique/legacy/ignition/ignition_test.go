package equation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/statistic"
)

var ignitionEpoch = time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)

/*
ignitionInput advances executed quantity, price, and time on a steady clock.
*/
func ignitionInput(index int) IgnitionInput {
	return IgnitionInput{
		Symbol: "BTC/USD",
		Volume: 20,
		Last:   100 + float64(index),
		Bid:    99.5 + float64(index),
		Ask:    100.5 + float64(index),
		At:     ignitionEpoch.Add(time.Duration(index) * time.Second),
	}
}

/*
warm drives enough steady observations to form empirical volume and move scales.
*/
func warm(ignition *Ignition, count int) (IgnitionOutput, bool, error) {
	var output IgnitionOutput
	var ready bool

	for index := range count {
		var err error
		output, ready, _, err = ignition.Measure(ignitionInput(index))

		if err != nil {
			return IgnitionOutput{}, false, err
		}
	}

	return output, ready, nil
}

/*
TestIgnition_Measure proves volume-clock sizing, empirical scale readiness,
causal ordering, held quote behavior, and bounded retention.
*/
func TestIgnition_Measure(t *testing.T) {
	Convey("Given invalid calculator or market inputs", t, func() {
		_, _, _, capacityErr := NewIgnition(0).Measure(ignitionInput(0))
		nonFinite := ignitionInput(0)
		nonFinite.Volume = math.Inf(1)
		_, _, _, inputErr := NewIgnition(8).Measure(nonFinite)

		Convey("It rejects them without installing fallback evidence", func() {
			So(capacityErr, ShouldNotBeNil)
			So(inputErr, ShouldNotBeNil)
		})
	})

	Convey("Given only the first closed empirical volume bar", t, func() {
		ignition := NewIgnition(128)
		_, _, _, err := ignition.Measure(ignitionInput(0))
		So(err, ShouldBeNil)
		output, ready, _, err := ignition.Measure(ignitionInput(1))

		Convey("Its dependent scores remain provisional and zero", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			So(output.RVOL, ShouldEqual, 0)
			So(output.Precursor, ShouldEqual, 0)
			So(output.Compression, ShouldEqual, 0)
			So(output.Ignition, ShouldEqual, 0)
			So(output.Trend, ShouldEqual, 0)
			So(output.Exhaustion, ShouldEqual, 0)
			So(output.Strength, ShouldEqual, 0)
			So(ignitionSquash(2, 0), ShouldEqual, 0)
			So(ignitionInverse(2, 0), ShouldEqual, 0)
		})

		Convey("A subsequent bar scores against the retained scales", func() {
			output, ready, _, err = ignition.Measure(ignitionInput(2))
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(output.RVOL, ShouldBeGreaterThan, 0)
			So(output.Precursor, ShouldBeGreaterThan, 0)
			So(output.Ignition, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given symbols trading on different quantity scales", t, func() {
		ignition := NewIgnition(128)
		var small IgnitionOutput
		var large IgnitionOutput

		for index := range 8 {
			at := ignitionEpoch.Add(time.Duration(index) * time.Second)
			var err error
			small, _, _, err = ignition.Measure(IgnitionInput{
				Symbol: "SMALL/USD",
				Volume: 20,
				Last:   100 + float64(index),
				Bid:    99.5 + float64(index),
				Ask:    100.5 + float64(index),
				At:     at,
			})
			So(err, ShouldBeNil)
			large, _, _, err = ignition.Measure(IgnitionInput{
				Symbol: "LARGE/USD",
				Volume: 200,
				Last:   1000 + float64(index*10),
				Bid:    995 + float64(index*10),
				Ask:    1005 + float64(index*10),
				At:     at,
			})
			So(err, ShouldBeNil)
		}

		Convey("Each target follows its own observed executed-volume advances", func() {
			So(ignition.windows["SMALL/USD"].deltas[0], ShouldEqual, 20)
			So(ignition.windows["LARGE/USD"].deltas[0], ShouldEqual, 200)
			So(small.RVOL, ShouldAlmostEqual, large.RVOL)
		})
	})

	Convey("Given repeated executed trades", t, func() {
		ignition := NewIgnition(128)
		_, _, err := warm(ignition, 12)
		So(err, ShouldBeNil)
		var output IgnitionOutput
		var ready bool

		for index := range 20 {
			output, ready, _, err = ignition.Measure(IgnitionInput{
				Symbol: "BTC/USD",
				Volume: 20,
				Last:   111 + float64(index%2),
				Bid:    110.5,
				Ask:    111.5,
				At:     ignitionEpoch.Add(12 * time.Second),
			})
			So(err, ShouldBeNil)
		}

		Convey("It retains finite empirical observations", func() {
			So(ready, ShouldBeTrue)
			So(output.Spread, ShouldEqual, 1.0)
			window := ignition.windows["BTC/USD"]

			for _, sample := range window.returns {
				So(sample, ShouldBeGreaterThanOrEqualTo, 0)
			}
		})
	})

	Convey("Given smaller trade quantity or regressing event time", t, func() {
		ignition := NewIgnition(128)
		_, _, err := warm(ignition, 4)
		So(err, ShouldBeNil)
		smaller := ignitionInput(4)
		smaller.Volume = 1
		_, _, _, volumeErr := ignition.Measure(smaller)
		regressed := ignitionInput(4)
		regressed.At = ignitionInput(2).At
		_, _, _, timeErr := ignition.Measure(regressed)

		Convey("It accepts quantity variation and rejects time regression", func() {
			So(volumeErr, ShouldBeNil)
			So(timeErr, ShouldNotBeNil)
		})
	})

	Convey("Given an invalid evidence combination", t, func() {
		_, err := ignitionMean(true, 1, math.Inf(1))

		Convey("It returns the probability failure instead of a silent zero", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given bounded empirical retention", t, func() {
		const capacity = 16
		ignition := NewIgnition(capacity)
		_, _, err := warm(ignition, 60)
		So(err, ShouldBeNil)

		Convey("No symbol history grows beyond the configured capacity", func() {
			window := ignition.windows["BTC/USD"]
			So(len(window.deltas), ShouldBeLessThanOrEqualTo, capacity)
			So(len(window.rates), ShouldBeLessThanOrEqualTo, capacity)
			So(len(window.returns), ShouldBeLessThanOrEqualTo, capacity)
			So(len(window.precursors), ShouldBeLessThanOrEqualTo, capacity)
			So(len(window.spreads), ShouldBeLessThanOrEqualTo, capacity)
		})
	})

	Convey("Given reciprocal multi-leg price paths on the same volume clock", t, func() {
		bullish := NewIgnition(128)
		bearish := NewIgnition(128)
		prices := []float64{100, 101, 100, 102, 101, 104}
		var bullishOutput IgnitionOutput
		var bearishOutput IgnitionOutput
		var priorPrecursors []float64

		for index, price := range prices {
			if index == len(prices)-1 {
				priorPrecursors = append(
					[]float64(nil),
					bullish.windows["BULL/USD"].precursors...,
				)
			}

			at := ignitionEpoch.Add(time.Duration(index) * time.Second)
			var err error
			bullishOutput, _, _, err = bullish.Measure(IgnitionInput{
				Symbol: "BULL/USD", Volume: 20, Last: price,
				Bid: price - 0.5, Ask: price + 0.5, At: at,
			})
			So(err, ShouldBeNil)
			reciprocal := 1 / price
			spread := reciprocal / 100
			bearishOutput, _, _, err = bearish.Measure(IgnitionInput{
				Symbol: "BEAR/USD", Volume: 20, Last: reciprocal,
				Bid: reciprocal - spread/2, Ask: reciprocal + spread/2, At: at,
			})
			So(err, ShouldBeNil)
		}

		Convey("Bullish evidence in price should equal bearish evidence in reciprocal price", func() {
			priorBaseline, ready := statistic.MedianOf(priorPrecursors)
			So(ready, ShouldBeTrue)
			expectedPrecursor := math.Log(104.0/101.0) / priorBaseline
			So(bullishOutput.Buy.Precursor,
				ShouldAlmostEqual, bearishOutput.Sell.Precursor, 1e-12)
			So(bullishOutput.Buy.Precursor,
				ShouldAlmostEqual, expectedPrecursor, 1e-12)
			So(bullishOutput.Buy.Ignition,
				ShouldAlmostEqual, bearishOutput.Sell.Ignition, 1e-12)
			So(bullishOutput.Buy.Exhaustion,
				ShouldAlmostEqual, bearishOutput.Sell.Exhaustion, 1e-12)
			So(bullishOutput.RVOL, ShouldAlmostEqual, bearishOutput.RVOL, 1e-12)

			for index, precursor := range bullish.windows["BULL/USD"].precursors {
				So(precursor,
					ShouldAlmostEqual, bearish.windows["BEAR/USD"].precursors[index], 1e-12)
			}
		})
	})
}

/*
BenchmarkIgnition_Measure measures the real incremental volume-clock path.
*/
func BenchmarkIgnition_Measure(b *testing.B) {
	ignition := NewIgnition(128)
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		if _, _, _, err := ignition.Measure(ignitionInput(index)); err != nil {
			b.Fatal(err)
		}

		index++
	}
}
