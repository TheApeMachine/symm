package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolPreviousValue   = types.MustIntern("temporal/previous_value")
	SymbolObservationSec  = types.MustIntern("temporal/observation_sec")
	SymbolObservationNsec = types.MustIntern("temporal/observation_nsec")
	SymbolObservations    = types.MustIntern("temporal/observations")
)

/*
Observer retains the previous value of one configured coordinate and exposes a
causal current/previous pair. One Observer belongs to one Stream; independent
series use independent keyed streams.
*/
func Observer(source types.Symbol) types.Primitive {
	return func(
		state types.Frame,
		input types.Frame,
	) (types.Frame, types.Frame, error) {
		current, hasCurrent := input.Get(source)
		seconds, hasSeconds := input.Get(SymbolUnixSec)
		nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

		if !hasCurrent || !hasSeconds || !hasNanoseconds ||
			nanoseconds < 0 || nanoseconds >= 1e9 {
			return state, types.Frame{}, fmt.Errorf(
				"temporal: observer requires a value and normalized event time",
			)
		}

		previous, hasPrevious := state.Get(SymbolPreviousValue)
		previousSeconds, hasPreviousSeconds := state.Get(SymbolObservationSec)
		previousNanoseconds, hasPreviousNanoseconds := state.Get(SymbolObservationNsec)

		if hasPreviousSeconds && hasPreviousNanoseconds {
			duration := seconds - previousSeconds +
				(nanoseconds-previousNanoseconds)*1e-9

			if duration < 0 {
				return state, types.Frame{}, fmt.Errorf(
					"temporal: observer event time must not regress",
				)
			}
		}

		observations, _ := state.Get(SymbolObservations)
		observations++
		nextState := state
		nextState.Put(SymbolPreviousValue, current)
		nextState.Put(SymbolObservationSec, seconds)
		nextState.Put(SymbolObservationNsec, nanoseconds)
		nextState.Put(SymbolObservations, observations)
		output := input
		output.Put(calculus.SymbolCurrent, current)
		output.Put(calculus.SymbolReady, 0)
		output.Put(SymbolObservations, observations)

		if hasPrevious {
			output.Put(calculus.SymbolPrevious, previous)
			output.Put(calculus.SymbolReady, 1)
			output.Put(SymbolObservedSec, previousSeconds)
			output.Put(SymbolObservedNsec, previousNanoseconds)
		}

		return nextState, output, nil
	}
}
