package advisor

import (
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
scrubFresh deletes every binding's Fresh marker from the frame ForkStrict hands
back. ForkStrict's output starts as a copy of its input and only overlays what
each branch newly wrote, so a marker present in the input survives untouched
unless something deletes it from the composed output directly. Without this
final scrub a Fresh bit set once would commit and read as fresh again on every
later call regardless of what that call actually delivered.
*/
func scrubFresh(bindings []MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		for _, binding := range bindings {
			input.Delete(binding.Fresh)
		}
	}
}

/*
freshTemporalContext returns the Window→ZScore→Baseline→Velocity composition
for one binding's series prefix, gated on that binding's Fresh marker: the
stage only advances the series when this call's own Measurement delivered the
value. When it did not, this is a deliberate no-op (the frame returns exactly
as given, no error).

It is used ONLY by advisors whose signal does not already publish a canonical
causal history for the composed quantity (MorphologyDynamics). Advisors whose
signals already emit baselines/z-scores/velocities compose those facts
directly instead of maintaining a second estimator.
*/
func freshTemporalContext(binding MetricBinding) nmtypes.Primitive {
	stage := nmtypes.Pipe(
		temporal.Window(binding.Prefix),
		statistic.ZScore(binding.Prefix),
		statistic.Baseline(binding.Prefix),
		statistic.Velocity(binding.Prefix),
	)

	return func(input *nmtypes.Frame) {
		if !input.Has(binding.Fresh) {
			return
		}

		stage(input)
	}
}
