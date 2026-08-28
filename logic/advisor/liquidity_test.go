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

		Convey("a call whose input never marks the binding Fresh fails without writing any slot", func() {
			// This is the shape TryFork relies on to drop a branch rather than
			// fail the whole composed step: the retained value/sec/nsec are
			// present (carried forward by Number.Step's merge), but Fresh is
			// not, because this call's own Measurement never delivered them.
			retained := nmtypes.Frame{}
			retained.Put(binding.Series.ValueSymbol, 0.01)
			retained.Put(binding.Series.SecSymbol, 0)
			retained.Put(binding.Series.NsecSymbol, 0)

			output := branch(retained)

			So(output.Err, ShouldNotBeNil)
			So(output.Mask, ShouldEqual, retained.Mask)
		})

		Convey("a Fresh call advances the retained series", func() {
			output := branch(freshFrame(binding, 0.01, 0, 0))

			So(output.Err, ShouldBeNil)
			So(binding.Series.Count(output), ShouldEqual, 1)
		})
	})

	Convey("Given the composed LiquidityPipeline over one binding", t, func() {
		binding := NewMetricBinding("liquidity", "relative_spread", "test/liquidity/pipeline_scrub")
		pipeline := LiquidityPipeline([]MetricBinding{binding})
		span := nmtypes.MustIntern("test/liquidity/pipeline_scrub/input/span")

		Convey("Fresh never survives into the committed output, even on a successful step", func() {
			frame := freshFrame(binding, 0.01, 0, 0)
			output := pipeline(frame)

			So(output.Err, ShouldBeNil)
			So(output.Has(binding.Fresh), ShouldBeFalse)
		})

		Convey("a genuinely fresh observation advances the retained ring exactly once, and a stale resubmission of the committed output does not advance it again", func() {
			// Force the window to retain more than one sample regardless of the
			// Baseline-fed span feedback, so a duplicate resubmission would be
			// observable in Count rather than masked by a capacity pinned at
			// one slot.
			seed := freshFrame(binding, 0.01, 0, 0)
			seed.Put(span, 4)
			first := pipeline(seed)
			So(first.Err, ShouldBeNil)
			So(binding.Series.Count(first), ShouldEqual, 1)

			second := freshFrame(binding, 0.02, 1, 0)
			second.Put(span, 4)
			merged := first
			merged.Merge(second)
			afterSecond := pipeline(merged)
			So(afterSecond.Err, ShouldBeNil)
			So(binding.Series.Count(afterSecond), ShouldEqual, 2)
			So(afterSecond.Has(binding.Fresh), ShouldBeFalse)

			// A stale resubmission: exactly the committed output carried
			// forward, precisely what Number.Step's merge would hand the
			// pipeline on the next call if some other, unrelated binding fired
			// instead of this one — no Fresh marker is set for this call.
			// TryFork forgives the branch (its own gate fails before writing
			// anything), so the composed step still succeeds overall, but the
			// series itself must not advance a second time for the same
			// observation.
			staleInput := nmtypes.Frame{}
			staleInput.Merge(afterSecond)
			stale := pipeline(staleInput)

			So(stale.Err, ShouldBeNil)
			So(binding.Series.Count(stale), ShouldEqual, 2)
		})
	})
}
