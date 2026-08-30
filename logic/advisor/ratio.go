package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Current-at and derived-provenance symbols. The advisor injects the current
event's observation time into the Frame before running the pipeline so a
cross-stream derived stage can reject future-leaked retained inputs and read
back each composed reading's own observation time. They are interned once here
because cross-stream composition depends on the same causal-alignment
discipline regardless of which family performs it.
*/
var (
	symbolCurrentAtSec  = nmtypes.MustIntern("advisor/current_at_unix_sec")
	symbolCurrentAtNsec = nmtypes.MustIntern("advisor/current_at_unix_nsec")
)

/*
ratioNamed declares one undefined-safe, future-leak-rejecting derived ratio:
out = numerator / denominator, in the ratios' own units. It reads the two
inputs' retained values and observation times from the named slots, compares
each input's own observation time against the current event time, and leaves
out UNSET (undefined) rather than erroring or fabricating a value when:

  - either input value is missing;
  - either input was observed at an instant after the current event (future
    leakage from a later producer-ring event);
  - the denominator is zero or non-finite;
  - the quotient itself is non-finite.

Definedness is the absence of the slot, matching the Perspective contract:
an undefined reading's zero Value is never mistaken for a real zero. Maturity
is derived as the minimum of the two inputs' maturities (the weakest estimator
support); SNR stays undeclared (no causal noise model applies to a
cross-stream ratio of already-measured facts).

Temporal provenance. ratioNamed also writes the derived reading's own temporal
coordinate so a consumer does not have to guess which interval the numerator
spanned nor which instant the ratio describes:

  - outFromSec/outFromNsec receive the NUMERATOR's interval start (copied from
    numeratorFromSec/numeratorFromNsec) — because the numerator is the interval
    quantity (accumulated flow) and its From is the honest interval origin of
    the joint reading.
  - outAtSec/outAtNsec receive the current event time — the evaluation instant,
    which is never allowed to precede any retained input (enforced below).

So two readings covering different interpreter intervals remain distinguishable
by their own From/ObservedAt, exactly as the Perspective contract requires; the
division is refused (out left unset) when either input is absent, future-
leaked, or the denominator is zero.
*/
func ratioNamed(
	numeratorValue, numeratorSec, numeratorNsec nmtypes.Symbol,
	numeratorFromSec, numeratorFromNsec nmtypes.Symbol,
	denominatorValue, denominatorSec, denominatorNsec nmtypes.Symbol,
	numeratorMaturity, denominatorMaturity nmtypes.Symbol,
	out, outMaturity nmtypes.Symbol,
	outFromSec, outFromNsec, outAtSec, outAtNsec nmtypes.Symbol,
) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		input.Delete(out)
		input.Delete(outFromSec)
		input.Delete(outFromNsec)
		input.Delete(outAtSec)
		input.Delete(outAtNsec)

		num, hasNum := input.Get(numeratorValue)
		numSec, hasNumSec := input.Get(numeratorSec)
		numNsec, _ := input.Get(numeratorNsec)
		den, hasDen := input.Get(denominatorValue)
		denSec, hasDenSec := input.Get(denominatorSec)
		denNsec, _ := input.Get(denominatorNsec)

		if !hasNum || !hasDen || !hasNumSec || !hasDenSec {
			return
		}

		curSec, hasCur := input.Get(symbolCurrentAtSec)
		curNsec, _ := input.Get(symbolCurrentAtNsec)

		if !hasCur {
			return
		}

		// Reject future-leaked retained inputs: a fact observed at an instant
		// after the event currently being evaluated is not causally available.
		if observedAfter(numSec, numNsec, curSec, curNsec) {
			return
		}

		if observedAfter(denSec, denNsec, curSec, curNsec) {
			return
		}

		if !utils.IsFinite(num) || !utils.IsFinite(den) || den == 0 {
			return
		}

		result := num / den

		if !utils.IsFinite(result) {
			return
		}

		input.Put(out, result)

		numMaturity, _ := input.Get(numeratorMaturity)
		denMaturity, _ := input.Get(denominatorMaturity)

		maturity := numMaturity

		if denMaturity < maturity {
			maturity = denMaturity
		}

		input.Put(outMaturity, maturity)

		// Carve the derived reading's temporal coordinate: interval origin from
		// the numerator, evaluation instant from the current event.
		if fromSec, hasFrom := input.Get(numeratorFromSec); hasFrom {
			fromNsec, _ := input.Get(numeratorFromNsec)
			input.Put(outFromSec, fromSec)
			input.Put(outFromNsec, fromNsec)
		}

		input.Put(outAtSec, curSec)
		input.Put(outAtNsec, curNsec)
	}
}

/*
observedAfter reports whether time (sec, nsec) is strictly later than
reference (refSec, refNsec).
*/
func observedAfter(sec, nsec, refSec, refNsec float64) bool {
	if sec != refSec {
		return sec > refSec
	}

	return nsec > refNsec
}

/*
jointFact relays one binding's projected raw value into that binding's output
slot, gated on Fresh. It is the joint-facts analogue of freshTemporalContext:
the metric's already-defined value is carried forward unchanged, with its own
provenance, rather than run through a derived-statistic stage. Shared by the
Execution Context and Decomposition families, which expose bound inputs as
named facts alongside their derived ratios.
*/
func jointFact(binding MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		if !input.Has(binding.Fresh) {
			return
		}

		value, found := input.Get(binding.Series.ValueSymbol)

		if !found {
			return
		}

		input.Put(binding.Series.ValueSymbol, value)
	}
}
