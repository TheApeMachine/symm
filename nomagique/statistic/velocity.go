package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolVelocityDelta        = nomagique.MustIntern("velocity/delta")
	SymbolVelocityAcceleration = nomagique.MustIntern("velocity/acceleration")
	SymbolVelocityElapsed      = nomagique.MustIntern("velocity/elapsed_sec")
	SymbolVelocityLastValue    = nomagique.MustIntern("velocity/last_value")
	SymbolVelocityLastDelta    = nomagique.MustIntern("velocity/last_delta")
	SymbolVelocityLastSec      = nomagique.MustIntern("velocity/last_sec")
	SymbolVelocityLastNsec     = nomagique.MustIntern("velocity/last_nsec")
)

/*
Velocity is the first difference of the observed value on the event clock,
with the second difference carried alongside. A departure shows in the
deltas long before it moves any baseline, so this primitive never smooths:
it reports the raw change and lets the composition normalize it. The first
observation seeds the differencer; the second produces a delta; the third
produces acceleration.
*/
func Velocity(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, hasValue := input.Get(nomagique.SampleValue)
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasValue || !hasSec || !hasNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: velocity requires a value and event time",
		)
	}

	if nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: velocity requires normalized nanoseconds",
		)
	}

	previousSec, hasLastSec := state.Get(SymbolVelocityLastSec)
	previousNsec, hasLastNsec := state.Get(SymbolVelocityLastNsec)

	if hasLastSec && hasLastNsec {
		if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
			return state, nomagique.Frame{}, fmt.Errorf(
				"statistic: velocity event time must not regress",
			)
		}
	}

	lastValue, hasLastValue := state.Get(SymbolVelocityLastValue)
	lastDelta, hasLastDelta := state.Get(SymbolVelocityLastDelta)

	nextState := state
	nextState.Put(SymbolVelocityLastValue, value)
	nextState.Put(SymbolVelocityLastSec, sec)
	nextState.Put(SymbolVelocityLastNsec, nsec)

	output := nomagique.Frame{}
	output.Put(nomagique.SampleValue, value)
	output.Put(SymbolReady, 0)

	if hasLastValue && hasLastSec && hasLastNsec {
		delta := value - lastValue
		elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)

		nextState.Put(SymbolVelocityLastDelta, delta)
		output.Put(SymbolVelocityDelta, delta)
		output.Put(SymbolVelocityElapsed, elapsed)
		output.Put(SymbolReady, 1)

		if hasLastDelta {
			output.Put(SymbolVelocityAcceleration, delta-lastDelta)
		}
	}

	return nextState, output, nil
}
