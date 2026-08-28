package advisor

import (
	"github.com/theapemachine/symm/nomagique/recurrence"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
HistoricalBindings declares the three complementary, already-standardized
trajectory dimensions the Historical Analogue Advisor composes for one symbol:

  - cvd/signed_net_fraction_zscore — executed-flow structure: aggressor-side
    imbalance, standardized against its own causal history by the CVD signal.
  - depthflow/book_imbalance_zscore — liquidity/book structure: displayed
    bid/ask asymmetry, standardized against its own causal history by the
    Depthflow signal.
  - hawkes/standardized_innovation:buy — event/excitation structure: the
    Hawkes point-process buy-side innovation, a z-score by construction
    ((N-Λ)/√Λ), standardized against the fitted conditional intensity.

Each metric is already a dimensionless, causally standardized quantity
published by its own signal (signal/README.md §12: a value is normalized only
when the normalization has an intrinsic mathematical interpretation), so no
further normalization happens inside this Advisor — recomputing a second
normalization universe here would silently redefine what "close" means
without any caller's knowledge. The three dimensions are genuinely
complementary structural families (flow, book, arrival-process) with no
redundancy between them, matching signal/README.md §10's standardized
multivariate trajectory Z_t = [z^(1)_t, ..., z^(k)_t].
*/
func HistoricalBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "signed_net_fraction_zscore", "advisor/historical/flow"),
		NewMetricBinding("depthflow", "book_imbalance_zscore", "advisor/historical/book"),
		NewMetricBinding("hawkes", "standardized_innovation:buy", "advisor/historical/excitation"),
	}
}

/*
HistoricalPipeline retains each bound dimension's own causal path and performs
one bounded, causal matrix-profile self-join across the joint trajectory —
signal/README.md §10's standardized-trajectory historical recurrence and
strategy/ADVISORS.md §10's Historical Analogue Perspective.

Each dimension's Path branch is gated on that binding's Fresh marker for
exactly the reason Liquidity's temporal-context branches are: Number.Step
merges the previous committed Frame under the incoming one before running the
pipeline, so without the marker a branch could not tell a fact this event
delivered from one merely retained from an earlier, unrelated event, and would
fabricate a duplicate observation on every other bound dimension's
Measurement. ForkStrict, not plain Fork, composes the branches for the same
reason established in LiquidityPipeline: two dimensions fresh on the same
Measurement each return a full copy of the shared input including the other's
already-populated prior state, and a blind sequential overlay would silently
revert whichever branch merges first back to its stale value.

recurrence.Analogue then runs against the always-retained joint state (never
gated on Fresh itself — a comparison over whatever has been retained so far is
exactly the current context, not a fact this specific event delivered), and a
final scrubFresh removes every marker before the frame can be committed.
*/
func HistoricalPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))
	prefixes := make([]string, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, freshPath(binding))
		prefixes = append(prefixes, binding.Prefix)
	}

	return nmtypes.Pipe(
		nmtypes.ForkStrict(branches...),
		recurrence.Analogue(prefixes...),
		scrubFresh(bindings),
	)
}

/*
HistoricalOutputs declares the six named facts HistoricalPipeline emits: the
joint comparison's nearest historical distance, its within-scan percentile,
the number of non-overlapping candidates actually searched, the nearest
match's start time, the query length the scan derived from retained history,
and the comparison's own maturity.

These are properties of the joint multivariate comparison, not of any single
bound dimension, so they cannot honestly borrow one binding's
Maturity/SNR/SNRDefined the way Liquidity's per-metric outputs do (a
derived output must represent its own provenance, not an arbitrary parent's).
Maturity is populated from recurrence.Analogue's own effective support (how
many historical candidates the nearest match was actually chosen among); SNR
has no principled definition for a nearest-neighbor distance (no causal noise
model applies here the way it does to a scalar departure), so it stays
honestly undefined rather than fabricated.
*/
func HistoricalOutputs() []Output {
	return []Output{
		NewDerivedOutput(recurrence.SymbolDistance, recurrence.SymbolMaturity),
		NewDerivedOutput(recurrence.SymbolPercentile, recurrence.SymbolMaturity),
		NewDerivedOutput(recurrence.SymbolMatchCount, recurrence.SymbolMaturity),
		NewDerivedOutput(recurrence.SymbolMatchFromSec, recurrence.SymbolMaturity),
		NewDerivedOutput(recurrence.SymbolMatchFromNsec, recurrence.SymbolMaturity),
		NewDerivedOutput(recurrence.SymbolQueryLength, recurrence.SymbolMaturity),
	}
}

/*
freshPath returns the temporal.Path retention primitive for one binding's
series prefix, gated on that binding's Fresh marker: the path only retains a
new observation when this call's own Measurement delivered the value.
*/
func freshPath(binding MetricBinding) nmtypes.Primitive {
	path := temporal.Path(binding.Prefix)

	return func(input nmtypes.Frame) nmtypes.Frame {
		if !input.Has(binding.Fresh) {
			return input
		}

		return path(input)
	}
}

/*
NewHistoricalAdvisor constructs the single concrete KindHistoricalAnalogue
Advisor instance over HistoricalPipeline and HistoricalBindings.
*/
func NewHistoricalAdvisor(name string) *Advisor {
	bindings := HistoricalBindings()

	return NewAdvisor(
		name,
		types.KindHistoricalAnalogue,
		HistoricalPipeline(bindings),
		bindings,
		HistoricalOutputs(),
	)
}
