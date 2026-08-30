package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestDecompositionBindings(t *testing.T) {
	Convey("Given DecompositionBindings", t, func() {
		bindings := DecompositionBindings()

		Convey("it binds frequency and throughput as two facts", func() {
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

		Convey("it carries frequency and throughput side by side without folding them", func() {
			frame := nmtypes.Frame{}

			// Give the two facts distinct values and mark both Fresh.
			frame.Put(bindings[0].Series.ValueSymbol, 12.0) // frequency
			frame.Put(bindings[0].Fresh, 1)
			frame.Put(bindings[1].Series.ValueSymbol, 40.0) // throughput
			frame.Put(bindings[1].Fresh, 1)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			freq, found := frame.Get(bindings[0].Series.ValueSymbol)
			So(found, ShouldBeTrue)
			So(freq, ShouldEqual, 12.0)

			thru, found := frame.Get(bindings[1].Series.ValueSymbol)
			So(found, ShouldBeTrue)
			So(thru, ShouldEqual, 40.0)

			// No Fresh marker survives.
			for _, binding := range bindings {
				So(frame.Has(binding.Fresh), ShouldBeFalse)
			}
		})
	})
}

func TestDecompositionOutputs(t *testing.T) {
	Convey("Given DecompositionOutputs", t, func() {
		bindings := DecompositionBindings()
		outputs := DecompositionOutputs(bindings)

		Convey("it declares one output per bound metric", func() {
			So(len(outputs), ShouldEqual, len(bindings))
		})

		Convey("each output borrows its metric's own provenance", func() {
			for index, output := range outputs {
				So(output.Maturity, ShouldEqual, bindings[index].Maturity)
			}
		})
	})
}
