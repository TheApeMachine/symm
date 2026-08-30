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

		Convey("it binds CVD's canonical mean trade notional and its coherent trade rate", func() {
			So(len(bindings), ShouldEqual, 2)

			metrics := map[string]bool{}
			sources := map[string]bool{}
			for _, binding := range bindings {
				metrics[binding.Metric] = true
				sources[binding.Source] = true
			}

			So(sources["cvd"], ShouldBeTrue)
			So(metrics["trade_rate"], ShouldBeTrue)
			So(metrics["mean_trade_notional"], ShouldBeTrue)

			// The cross-signal Hawkes arrival_rate is NOT consumed: dividing
			// CVD's gross-notional rate by Hawkes' arrival rate would equate
			// two possibly different retained populations.
			So(sources["hawkes"], ShouldBeFalse)
			So(metrics["arrival_rate"], ShouldBeFalse)
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

		frequency := bindings[0]
		meanSize := bindings[1]

		Convey("it relays the canonical mean trade notional and trade rate unchanged", func() {
			frame := nmtypes.Frame{}

			frame.Put(frequency.Series.ValueSymbol, 10.0) // events/sec
			frame.Put(frequency.Fresh, 1)
			frame.Put(meanSize.Series.ValueSymbol, 100.0) // quote/event
			frame.Put(meanSize.Fresh, 1)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			freq, foundFreq := frame.Get(frequency.Series.ValueSymbol)
			So(foundFreq, ShouldBeTrue)
			So(freq, ShouldAlmostEqual, 10.0, 1e-9)

			size, foundSize := frame.Get(meanSize.Series.ValueSymbol)
			So(foundSize, ShouldBeTrue)
			So(size, ShouldAlmostEqual, 100.0, 1e-9)

			// No Fresh marker survives.
			for _, binding := range bindings {
				So(frame.Has(binding.Fresh), ShouldBeFalse)
			}
		})

		Convey("it performs no division; only the two bound slots are touched", func() {
			frame := nmtypes.Frame{}
			frame.Put(frequency.Series.ValueSymbol, 10.0)
			frame.Put(frequency.Fresh, 1)
			frame.Put(meanSize.Series.ValueSymbol, 100.0)
			frame.Put(meanSize.Fresh, 1)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)
			// DecompositionOutputs declares exactly two outputs; the pipeline
			// writes no additional derived slot, so there is no third quantity.
		})
	})
}

func TestDecompositionOutputs(t *testing.T) {
	Convey("Given DecompositionOutputs", t, func() {
		bindings := DecompositionBindings()
		outputs := DecompositionOutputs(bindings)

		Convey("it exposes exactly the canonical two facts, no derived quantity", func() {
			So(len(outputs), ShouldEqual, 2)
			So(outputs[0].Slot, ShouldEqual, bindings[0].Series.ValueSymbol)
			So(outputs[1].Slot, ShouldEqual, bindings[1].Series.ValueSymbol)
		})
	})
}

/*
TestDecompositionUsesCanonicalMeanTradeNotional asserts that the decomposition
advisor surfaces CVD's own mean_trade_notional (G/N) — the quantity its spec
already defines for many-small vs few-large — rather than manufacturing a
cross-horizon duplicate. Two CVD measurements with the same mean trade notional
but different retained horizons must report the same mean size, because the
quantity is already horizon-normalized inside the signal.
*/
func TestDecompositionUsesCanonicalMeanTradeNotional(t *testing.T) {
	Convey("Given a decomposition advisor fed a CVD measurement", t, func() {
		advisor := NewDecompositionAdvisor("advisor.decomposition.canonical:" + t.Name())
		at := time.Unix(100, 0)

		perspective := advisor.Step(testMeasurement("TEST/USD", "cvd", at, map[string]float64{
			"trade_rate":        10.0,
			"mean_trade_notional": 100.0,
		}))

		So(perspective, ShouldNotBeNil)
		So(perspective.Err, ShouldBeNil)

		bindings := DecompositionBindings()
		meanReading, found := readingFor(perspective, bindings[1].Series.ValueSymbol)
		So(found, ShouldBeTrue)
		So(meanReading.Defined, ShouldBeTrue)
		So(meanReading.Value, ShouldAlmostEqual, 100.0, 1e-9)

		freqReading, freqFound := readingFor(perspective, bindings[0].Series.ValueSymbol)
		So(freqFound, ShouldBeTrue)
		So(freqReading.Value, ShouldAlmostEqual, 10.0, 1e-9)

		Convey("there is no fabricated derived slot beyond the two canonical facts", func() {
			So(perspective.Count, ShouldEqual, 2)
		})
	})
}
