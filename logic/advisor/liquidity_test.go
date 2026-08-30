package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
freshFrame builds one incoming Frame for a single binding, marking it Fresh so
its branch is allowed to advance.
*/
func freshFrame(binding MetricBinding, value, sec, nsec float64) nmtypes.Frame {
	frame := nmtypes.Frame{}
	frame.Put(binding.Series.ValueSymbol, value)
	frame.Put(binding.Series.SecSymbol, sec)
	frame.Put(binding.Series.NsecSymbol, nsec)
	frame.Put(binding.Fresh, 1)

	return frame
}

func TestFreshTemporalContext(t *testing.T) {
	Convey("Given one binding's guarded temporal-context branch in isolation", t, func() {
		binding := NewMetricBinding("liquidity", "relative_spread", "test/liquidity/fresh_branch")
		branch := freshTemporalContext(binding)

		Convey("a call whose input never marks the binding Fresh is a deliberate no-op, never an error", func() {
			// The retained value/sec/nsec are present (carried forward by
			// Number.Step's merge), but Fresh is not, because this call's own
			// Measurement never delivered them. Fresh already says everything
			// there is to say about whether this branch applies to this
			// event — the branch must not turn that into an error for Fork
			// (or anything else) to interpret.
			retained := nmtypes.Frame{}
			retained.Put(binding.Series.ValueSymbol, 0.01)
			retained.Put(binding.Series.SecSymbol, 0)
			retained.Put(binding.Series.NsecSymbol, 0)

			original := retained
			output := retained
			branch(&output)

			So(output.Err, ShouldBeNil)
			So(output.Equal(original), ShouldBeTrue)
		})

		Convey("a Fresh call advances the retained series", func() {
			output := freshFrame(binding, 0.01, 0, 0)
			branch(&output)

			So(output.Err, ShouldBeNil)
			So(binding.Series.Count(output), ShouldEqual, 1)
		})

		Convey("a Fresh call that is itself malformed propagates its error unconditionally, never mistaken for absence", func() {
			// Fresh is set, so this is unambiguously "this call's own
			// Measurement delivered this metric" — the stage's own event-time
			// regression guard rejecting it is a genuine defect that must
			// surface, not something a composition layer could ever
			// legitimately reinterpret as "not yet observed."
			first := freshFrame(binding, 0.01, 5, 0)
			branch(&first)
			So(first.Err, ShouldBeNil)

			merged := first
			merged.Merge(freshFrame(binding, 0.02, 1, 0))
			branch(&merged)

			So(merged.Err, ShouldNotBeNil)
		})
	})

	Convey("Given the composed MorphologyDynamicsPipeline over one binding", t, func() {
		binding := NewMetricBinding("morphology", "morphology_change", "test/morphology_dynamics/pipeline_scrub")
		pipeline := MorphologyDynamicsPipeline([]MetricBinding{binding})
		span := nmtypes.MustIntern("test/morphology_dynamics/pipeline_scrub/input/span")

		Convey("Fresh never survives into the committed output, even on a successful step", func() {
			output := freshFrame(binding, 0.01, 0, 0)
			pipeline(&output)

			So(output.Err, ShouldBeNil)
			So(output.Has(binding.Fresh), ShouldBeFalse)
		})

		Convey("a genuinely fresh observation advances the retained ring exactly once, and a stale resubmission of the committed output does not advance it again", func() {
			// Force the window to retain more than one sample regardless of the
			// Baseline-fed span feedback, so a duplicate resubmission would be
			// observable in Count rather than masked by a capacity pinned at
			// one slot.
			first := freshFrame(binding, 0.01, 0, 0)
			first.Put(span, 4)
			pipeline(&first)
			So(first.Err, ShouldBeNil)
			So(binding.Series.Count(first), ShouldEqual, 1)

			second := freshFrame(binding, 0.02, 1, 0)
			second.Put(span, 4)
			afterSecond := first
			afterSecond.Merge(second)
			pipeline(&afterSecond)
			So(afterSecond.Err, ShouldBeNil)
			So(binding.Series.Count(afterSecond), ShouldEqual, 2)
			So(afterSecond.Has(binding.Fresh), ShouldBeFalse)

			// A stale resubmission: exactly the committed output carried
			// forward, precisely what Number.Step's merge would hand the
			// pipeline on the next call if some other, unrelated binding fired
			// instead of this one — no Fresh marker is set for this call. The
			// branch's own gate makes this a deliberate no-op, so the composed
			// step still succeeds overall, but the series itself must not
			// advance a second time for the same observation.
			stale := nmtypes.Frame{}
			stale.Merge(afterSecond)
			pipeline(&stale)

			So(stale.Err, ShouldBeNil)
			So(binding.Series.Count(stale), ShouldEqual, 2)
		})
	})
}
