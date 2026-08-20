package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolResidual              = nomagique.MustIntern("z/residual")
	SymbolDispersion            = nomagique.MustIntern("z/dispersion")
	SymbolZScore                = nomagique.MustIntern("z/value")
	SymbolDispersionLastSec     = nomagique.MustIntern("z/last_sec")
	SymbolDispersionLastNsec    = nomagique.MustIntern("z/last_nsec")
	SymbolDispersionHalflife    = nomagique.MustIntern("z/dispersion_halflife_sec")
)

/*
ZScore measures how far the observed value stands from the composed
baseline, in units of the residuals' own event-time decayed dispersion. The
dispersion is a decayed root mean square of residuals, so it tracks the
variance around the baseline as it is right now instead of assuming a static
spread, and the score is comparable across every series because each series
normalizes by itself. Until the composition has produced a baseline there is
no residual to measure, and the primitive reports not ready.
*/
func ZScore(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, hasValue := input.Get(nomagique.SampleValue)
	halflife, hasHalflife := input.Get(SymbolDispersionHalflife)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasValue || !hasHalflife || !hasSec || !hasNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: z-score requires a value, a dispersion halflife, and event time",
		)
	}

	if halflife <= 0 || nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: z-score requires a positive dispersion halflife and normalized nanoseconds",
		)
	}

	baseline, hasBaseline := state.Get(SymbolBaselineValue)

	if !hasBaseline {
		output := input
		output.Put(nomagique.SampleValue, value)
		output.Put(SymbolReady, 0)

		return state, output, nil
	}

	previousSec, hasLastSec := state.Get(SymbolDispersionLastSec)
	previousNsec, hasLastNsec := state.Get(SymbolDispersionLastNsec)

	if hasLastSec && hasLastNsec {
		if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
			return state, nomagique.Frame{}, fmt.Errorf(
				"statistic: z-score event time must not regress",
			)
		}
	}

	residual := value - baseline
	nextState := state
	dispersion, hasDispersion := state.Get(SymbolDispersion)

	if !hasDispersion || !hasLastSec || !hasLastNsec {
		nextState.Put(SymbolDispersion, math.Abs(residual))
	} else {
		elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)
		alpha := 1 - math.Exp(-elapsed*math.Ln2/halflife)
		energy := dispersion * dispersion
		energy += alpha * (residual*residual - energy)
		nextState.Put(SymbolDispersion, math.Sqrt(energy))
	}

	nextState.Put(SymbolDispersionLastSec, sec)
	nextState.Put(SymbolDispersionLastNsec, nsec)

	currentDispersion, _ := nextState.Get(SymbolDispersion)
	score := 0.0

	if currentDispersion > 0 {
		score = residual / currentDispersion
	}

	output := input
	output.Put(nomagique.SampleValue, value)
	output.Put(SymbolResidual, residual)
	output.Put(SymbolDispersion, currentDispersion)
	output.Put(SymbolZScore, score)
	output.Put(SymbolReady, 1)

	return nextState, output, nil
}
