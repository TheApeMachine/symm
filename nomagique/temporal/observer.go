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
Observer returns the primitive that retains the previous value of one
configured coordinate and exposes a causal current/previous pair. The prefix
namespaces its state slots, so one frame can carry several independent
observers.
*/
func Observer(prefix string, source types.Symbol) types.Primitive {
	previousValue := types.MustIntern(JoinPrefix(prefix, "temporal/previous_value"))
	observationSec := types.MustIntern(JoinPrefix(prefix, "temporal/observation_sec"))
	observationNsec := types.MustIntern(JoinPrefix(prefix, "temporal/observation_nsec"))
	observations := types.MustIntern(JoinPrefix(prefix, "temporal/observations"))
	observedSec := types.MustIntern(JoinPrefix(prefix, "temporal/observed_sec"))
	observedNsec := types.MustIntern(JoinPrefix(prefix, "temporal/observed_nsec"))

	return func(input types.Frame) types.Frame {
		current, hasCurrent := input.Get(source)
		seconds, hasSeconds := input.Get(SymbolUnixSec)
		nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

		if !hasCurrent || !hasSeconds || !hasNanoseconds ||
			nanoseconds < 0 || nanoseconds >= 1e9 {
			input.Err = fmt.Errorf(
				"temporal: observer requires a value and normalized event time",
			)

			return input
		}

		previous, hasPrevious := input.Get(previousValue)
		previousSeconds, hasPreviousSeconds := input.Get(observationSec)
		previousNanoseconds, hasPreviousNanoseconds := input.Get(observationNsec)

		if hasPreviousSeconds && hasPreviousNanoseconds {
			duration := seconds - previousSeconds +
				(nanoseconds-previousNanoseconds)*1e-9

			if duration < 0 {
				input.Err = fmt.Errorf(
					"temporal: observer event time must not regress",
				)

				return input
			}
		}

		observationCount, _ := input.Get(observations)
		observationCount++
		input.Put(previousValue, current)
		input.Put(observationSec, seconds)
		input.Put(observationNsec, nanoseconds)
		input.Put(observations, observationCount)
		input.Put(calculus.SymbolCurrent, current)
		input.Put(calculus.SymbolReady, 0)

		if hasPrevious {
			input.Put(calculus.SymbolPrevious, previous)
			input.Put(calculus.SymbolReady, 1)
			input.Put(observedSec, previousSeconds)
			input.Put(observedNsec, previousNanoseconds)
		}

		return input
	}
}
