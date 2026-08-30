package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestDecompositionBindings(t *testing.T) {
	Convey("Given DecompositionBindings", t, func() {
		bindings := DecompositionBindings()

		Convey("it binds frequency and throughput", func() {
			So(len(bindings), ShouldEqual, 2)

			metrics := map[string]bool{}
			for _, binding := range bindings {
				metrics[binding.Metric] = true
			}

			So(metrics["arrival_rate"], ShouldBeTrue)
			So(metrics["gross_notional_rate"], ShouldBeTrue)
		})

		Convey("each binding has a unique prefix", func() {
			prefixes := map[string]bool{}
			for _, binding := range bindings {
				prefixes[binding.Prefix] = true
			}

			So(len(prefixes), ShouldEqual, len(bindings))
		})
	})
}

func TestDecompositionPipeline(t *testing.T) {
	Convey("Given DecompositionPipeline", t, func() {
		bindings := DecompositionBindings()
		pipeline := DecompositionPipeline(bindings)

		freq := bindings[0]
		throughput := bindings[1]

		Convey("the hand-calculated mean event size is exact", func() {
			// arrival_rate = 10 events/sec, gross_notional_rate = 1000 quote/sec
			// => mean event size = 1000 / 10 = 100 quote/event.
			frame := nmtypes.Frame{}
			frame.Put(symbolCurrentAtSec, 100)
			frame.Put(symbolCurrentAtNsec, 0)

			frame.Put(throughput.Series.ValueSymbol, 1000.0)
			frame.Put(throughput.Series.SecSymbol, 100)
			frame.Put(throughput.Series.NsecSymbol, 0)
			frame.Put(throughput.Fresh, 1)
			frame.Put(throughput.Maturity, 0.9)

			frame.Put(freq.Series.ValueSymbol, 10.0)
			frame.Put(freq.Series.SecSymbol, 100)
			frame.Put(freq.Series.NsecSymbol, 0)
			frame.Put(freq.Fresh, 1)
			frame.Put(freq.Maturity, 0.8)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			mean, defined := frame.Get(symbolDecompMeanNotional)
			So(defined, ShouldBeTrue)
			So(mean, ShouldAlmostEqual, 100.0, 1e-9)
		})

		Convey("zero arrival rate leaves mean event size undefined, not zero or infinity", func() {
			frame := nmtypes.Frame{}
			frame.Put(symbolCurrentAtSec, 100)
			frame.Put(symbolCurrentAtNsec, 0)

			frame.Put(throughput.Series.ValueSymbol, 1000.0)
			frame.Put(throughput.Series.SecSymbol, 100)
			frame.Put(throughput.Series.NsecSymbol, 0)
			frame.Put(throughput.Fresh, 1)
			frame.Put(throughput.Maturity, 0.9)

			frame.Put(freq.Series.ValueSymbol, 0.0)
			frame.Put(freq.Series.SecSymbol, 100)
			frame.Put(freq.Series.NsecSymbol, 0)
			frame.Put(freq.Fresh, 1)
			frame.Put(freq.Maturity, 0.8)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			_, defined := frame.Get(symbolDecompMeanNotional)
			So(defined, ShouldBeFalse)
		})

		Convey("mutating either input breaks the derived quantity", func() {
			base := nmtypes.Frame{}
			base.Put(symbolCurrentAtSec, 100)
			base.Put(symbolCurrentAtNsec, 0)
			base.Put(throughput.Series.ValueSymbol, 1000.0)
			base.Put(throughput.Series.SecSymbol, 100)
			base.Put(throughput.Series.NsecSymbol, 0)
			base.Put(throughput.Fresh, 1)
			base.Put(throughput.Maturity, 0.9)
			base.Put(freq.Series.ValueSymbol, 10.0)
			base.Put(freq.Series.SecSymbol, 100)
			base.Put(freq.Series.NsecSymbol, 0)
			base.Put(freq.Fresh, 1)
			base.Put(freq.Maturity, 0.8)

			reference := nmtypes.Frame{}
			reference.Merge(base)
			pipeline(&reference)
			refMean, _ := reference.Get(symbolDecompMeanNotional)

			// Mutate throughput.
			mutatedThroughput := nmtypes.Frame{}
			mutatedThroughput.Merge(base)
			mutatedThroughput.Put(throughput.Series.ValueSymbol, 2000.0)
			pipeline(&mutatedThroughput)
			throughputMean, _ := mutatedThroughput.Get(symbolDecompMeanNotional)
			So(throughputMean, ShouldNotAlmostEqual, refMean, 1e-9)

			// Mutate arrival rate.
			mutatedArrival := nmtypes.Frame{}
			mutatedArrival.Merge(base)
			mutatedArrival.Put(freq.Series.ValueSymbol, 20.0)
			pipeline(&mutatedArrival)
			arrivalMean, _ := mutatedArrival.Get(symbolDecompMeanNotional)
			So(arrivalMean, ShouldNotAlmostEqual, refMean, 1e-9)
		})

		Convey("future-leaked denominator leaves the derived quantity undefined", func() {
			// The denominator (arrival_rate) is retained from a LATER event
			// (t=200) while the current evaluation is at t=100. The joint must
			// not use it.
			frame := nmtypes.Frame{}
			frame.Put(symbolCurrentAtSec, 100)
			frame.Put(symbolCurrentAtNsec, 0)

			frame.Put(throughput.Series.ValueSymbol, 1000.0)
			frame.Put(throughput.Series.SecSymbol, 100)
			frame.Put(throughput.Series.NsecSymbol, 0)
			frame.Put(throughput.Fresh, 1)
			frame.Put(throughput.Maturity, 0.9)

			// Denom retained at t=200 > current 100.
			frame.Put(freq.Series.ValueSymbol, 10.0)
			frame.Put(freq.Series.SecSymbol, 200)
			frame.Put(freq.Series.NsecSymbol, 0)
			frame.Put(freq.Fresh, 1)
			frame.Put(freq.Maturity, 0.8)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			_, defined := frame.Get(symbolDecompMeanNotional)
			So(defined, ShouldBeFalse)
		})
	})
}

func TestDecompositionOutputs(t *testing.T) {
	Convey("Given DecompositionOutputs", t, func() {
		bindings := DecompositionBindings()
		outputs := DecompositionOutputs(bindings)

		Convey("it declares the derived mean event size plus the two inputs", func() {
			So(len(outputs), ShouldEqual, 3)
			So(outputs[0].Slot, ShouldEqual, symbolDecompMeanNotional)
		})

		Convey("the derived mean borrows its own maturity slot", func() {
			So(outputs[0].Maturity, ShouldEqual, symbolDecompMeanMatur)
		})
	})
}

/*
TestDecompositionMeanEventSizeEndToEnd asserts the full advisor path produces
the hand-calculated mean event size, not merely a side-by-side relay.
*/
func TestDecompositionMeanEventSizeEndToEnd(t *testing.T) {
	Convey("Given a decomposition advisor fed frequency then throughput", t, func() {
		advisor := NewDecompositionAdvisor("advisor.decomposition.e2e:" + t.Name())
		at := time.Unix(100, 0)

		advisor.Step(testMeasurement("TEST/USD", "hawkes", at, map[string]float64{
			"arrival_rate": 10.0,
		}))

		perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
			"gross_notional_rate": 1000.0,
		}))

		So(perspective, ShouldNotBeNil)
		So(perspective.Err, ShouldBeNil)

		mean, found := readingFor(perspective, symbolDecompMeanNotional)
		So(found, ShouldBeTrue)
		So(mean.Defined, ShouldBeTrue)
		So(mean.Value, ShouldAlmostEqual, 100.0, 1e-9)

		// The two input facts are still exposed with their own provenance.
		freqReading, freqFound := readingFor(perspective, bindingsArrivalRateSlot())
		So(freqFound, ShouldBeTrue)
		So(freqReading.Defined, ShouldBeTrue)
		So(freqReading.Value, ShouldAlmostEqual, 10.0, 1e-9)
	})
}

// bindingsArrivalRateSlot returns the interned slot for the arrival_rate input
// fact in the current DecompositionBindings ordering.
func bindingsArrivalRateSlot() nmtypes.Symbol {
	return DecompositionBindings()[0].Series.ValueSymbol
}
