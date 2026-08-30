package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestExecutionBindings(t *testing.T) {
	Convey("Given ExecutionBindings", t, func() {
		bindings := ExecutionBindings()

		Convey("it declares distinct, uniquely-prefixed series", func() {
			prefixes := make(map[string]bool)
			for _, binding := range bindings {
				prefixes[binding.Prefix] = true
			}

			So(len(prefixes), ShouldEqual, len(bindings))
			So(len(bindings), ShouldEqual, 3)
		})

		Convey("it binds flow + two capacity facts", func() {
			sources := map[string]bool{}
			metrics := map[string]bool{}
			for _, binding := range bindings {
				sources[binding.Source] = true
				metrics[binding.Metric] = true
			}

			So(sources["cvd"], ShouldBeTrue)
			So(sources["liquidity"], ShouldBeTrue)
			So(metrics["signed_net_fraction_zscore"], ShouldBeTrue)
			So(metrics["touch_notional_imbalance"], ShouldBeTrue)
			So(metrics["relative_spread"], ShouldBeTrue)
		})
	})
}

func TestExecutionPipeline(t *testing.T) {
	Convey("Given ExecutionPipeline", t, func() {
		bindings := ExecutionBindings()
		pipeline := ExecutionPipeline(bindings)

		Convey("a Fresh joint-facts call carries each value into its own slot without multiplying", func() {
			frame := nmtypes.Frame{}

			for _, binding := range bindings {
				frame.Put(binding.Series.ValueSymbol, 1.5)
				frame.Put(binding.Fresh, 1)
			}

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)

			// Every output slot holds its own bound metric's value, unchanged —
			// the pipeline is identity-per-binding, never a compound scalar.
			for _, binding := range bindings {
				value, found := frame.Get(binding.Series.ValueSymbol)
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 1.5)
			}

			// No Fresh marker survives into the committed state.
			for _, binding := range bindings {
				So(frame.Has(binding.Fresh), ShouldBeFalse)
			}
		})

		Convey("a not-Fresh binding is left untouched", func() {
			frame := nmtypes.Frame{}
			first := bindings[0]
			frame.Put(first.Series.ValueSymbol, 2.0)
			frame.Put(first.Fresh, 1)

			pipeline(&frame)

			So(frame.Err, ShouldBeNil)
			value, found := frame.Get(first.Series.ValueSymbol)
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 2.0)
		})
	})
}

func TestExecutionOutputs(t *testing.T) {
	Convey("Given ExecutionOutputs", t, func() {
		bindings := ExecutionBindings()
		outputs := ExecutionOutputs(bindings)

		Convey("it declares exactly one output per bound metric", func() {
			So(len(outputs), ShouldEqual, len(bindings))
			So(len(outputs), ShouldEqual, 3)
		})

		Convey("each output borrows its own metric's provenance", func() {
			for index, output := range outputs {
				binding := bindings[index]
				So(output.Maturity, ShouldEqual, binding.Maturity)
				So(output.SNR, ShouldEqual, binding.SNR)
			}
		})
	})
}
