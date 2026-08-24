package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolResidual           = types.MustIntern("z/residual")
	SymbolDispersion         = types.MustIntern("z/dispersion")
	SymbolZScore             = types.MustIntern("z/value")
	SymbolDispersionLastSec  = types.MustIntern("z/last_sec")
	SymbolDispersionLastNsec = types.MustIntern("z/last_nsec")
	SymbolDispersionHalflife = types.MustIntern("z/dispersion_halflife_sec")
)

type zScoreSlots struct {
	residual      types.Symbol
	dispersion    types.Symbol
	value         types.Symbol
	lastSec       types.Symbol
	lastNsec      types.Symbol
	halflife      types.Symbol
	baselineValue types.Symbol
	baselineSpan  types.Symbol
}

func newZScoreSlots(prefix string) zScoreSlots {
	baseline := newBaselineSlots(prefix)

	return zScoreSlots{
		residual:      types.MustIntern(temporal.JoinPrefix(prefix, "z/residual")),
		dispersion:    types.MustIntern(temporal.JoinPrefix(prefix, "z/dispersion")),
		value:         types.MustIntern(temporal.JoinPrefix(prefix, "z/value")),
		lastSec:       types.MustIntern(temporal.JoinPrefix(prefix, "z/last_sec")),
		lastNsec:      types.MustIntern(temporal.JoinPrefix(prefix, "z/last_nsec")),
		halflife:      types.MustIntern(temporal.JoinPrefix(prefix, "z/dispersion_halflife_sec")),
		baselineValue: baseline.value,
		baselineSpan:  baseline.span,
	}
}

/*
ZScore returns the primitive that measures how far the observed value stands
from the composed baseline of the same series, in units of the residuals' own
event-time decayed dispersion. The prefix namespaces every slot; the empty
prefix keeps the legacy generic slots.

The dispersion is a decayed root mean square of residuals, so it tracks the
variance around the baseline as it is right now instead of assuming a static
spread, and the score is comparable across every series because each series
normalizes by itself. Until the composition has produced a baseline there is
no residual to measure, and the primitive reports not ready.
*/
func ZScore(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	slots := newZScoreSlots(prefix)

	return func(input types.Frame) types.Frame {
		value, hasValue := input.Get(series.ValueSymbol)
		sec, hasSec := input.Get(series.SecSymbol)
		nsec, hasNsec := input.Get(series.NsecSymbol)

		if !hasValue || !hasSec || !hasNsec {
			input.Err = fmt.Errorf(
				"statistic: z-score requires a value and event time",
			)

			return input
		}

		if nsec < 0 || nsec >= 1e9 {
			input.Err = fmt.Errorf(
				"statistic: z-score requires normalized nanoseconds",
			)

			return input
		}

		halflife, hasHalflife := input.Get(slots.halflife)

		if hasHalflife && halflife < 0 {
			input.Err = fmt.Errorf(
				"statistic: z-score requires a non-negative dispersion halflife",
			)

			return input
		}

		if !hasHalflife || halflife == 0 {
			halflife = 1.0

			if span, hasSpan := input.Get(slots.baselineSpan); hasSpan && span > 0 {
				halflife = span
			}
		}

		baseline, hasBaseline := input.Get(slots.baselineValue)

		if !hasBaseline {
			input.Put(series.ValueSymbol, value)
			input.Put(series.ReadySymbol, 0)

			return input
		}

		previousSec, hasLastSec := input.Get(slots.lastSec)
		previousNsec, hasLastNsec := input.Get(slots.lastNsec)

		if hasLastSec && hasLastNsec {
			if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
				input.Err = fmt.Errorf(
					"statistic: z-score event time must not regress",
				)

				return input
			}
		}

		residual := value - baseline
		dispersion, hasDispersion := input.Get(slots.dispersion)
		updatedDispersion := math.Abs(residual)

		if hasDispersion && hasLastSec && hasLastNsec {
			elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)
			alpha := 1 - math.Exp(-elapsed*math.Ln2/halflife)
			energy := dispersion * dispersion
			energy += alpha * (residual*residual - energy)
			updatedDispersion = math.Sqrt(energy)
		}

		input.Put(slots.lastSec, sec)
		input.Put(slots.lastNsec, nsec)
		input.Put(slots.dispersion, updatedDispersion)

		score := 0.0

		if updatedDispersion > 0 {
			score = residual / updatedDispersion
		}

		input.Put(series.ValueSymbol, value)
		input.Put(slots.residual, residual)
		input.Put(slots.value, score)
		input.Put(series.ReadySymbol, 1)

		return input
	}
}
