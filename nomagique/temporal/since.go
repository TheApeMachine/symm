package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolOriginSec      = types.MustIntern("temporal/origin_sec")
	SymbolOriginNsec     = types.MustIntern("temporal/origin_nsec")
	SymbolObservedSec    = types.MustIntern("temporal/observed_sec")
	SymbolObservedNsec   = types.MustIntern("temporal/observed_nsec")
	SymbolAdvanced       = types.MustIntern("temporal/advanced")
	SymbolCompletedSpans = types.MustIntern("temporal/completed_spans")
)

/*
Since measures elapsed event time from a retained origin. The first event
establishes the origin; subsequent events expose the same origin until Restart
explicitly begins a new span.
*/
func Since(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	seconds, hasSeconds := input.Get(SymbolUnixSec)
	nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

	if !hasSeconds || !hasNanoseconds || nanoseconds < 0 || nanoseconds >= 1e9 {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: since requires normalized event-time coordinates",
		)
	}

	originSeconds, hasOriginSeconds := state.Get(SymbolOriginSec)
	originNanoseconds, hasOriginNanoseconds := state.Get(SymbolOriginNsec)
	nextState := state

	if !hasOriginSeconds || !hasOriginNanoseconds {
		originSeconds = seconds
		originNanoseconds = nanoseconds
		nextState.Put(SymbolOriginSec, originSeconds)
		nextState.Put(SymbolOriginNsec, originNanoseconds)
	}

	duration := seconds - originSeconds +
		(nanoseconds-originNanoseconds)*1e-9

	if duration < 0 {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: since event time must not precede its origin",
		)
	}

	advanced := 0.0

	if duration > 0 {
		advanced = 1
	}

	output := input
	output.Put(calculus.SymbolDuration, duration)
	output.Put(SymbolObservedSec, originSeconds)
	output.Put(SymbolObservedNsec, originNanoseconds)
	output.Put(SymbolAdvanced, advanced)
	completed, _ := state.Get(SymbolCompletedSpans)
	output.Put(SymbolCompletedSpans, completed)

	return nextState, output, nil
}

/*
Restart moves the retained origin to the current event and counts one completed
span. It preserves the output produced for the span that just ended.
*/
func Restart(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	seconds, hasSeconds := input.Get(SymbolUnixSec)
	nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

	if !hasSeconds || !hasNanoseconds || nanoseconds < 0 || nanoseconds >= 1e9 {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: restart requires normalized event-time coordinates",
		)
	}

	completed, _ := state.Get(SymbolCompletedSpans)
	nextState := state
	nextState.Put(SymbolOriginSec, seconds)
	nextState.Put(SymbolOriginNsec, nanoseconds)
	nextState.Put(SymbolCompletedSpans, completed+1)
	output := input
	output.Put(SymbolCompletedSpans, completed+1)

	return nextState, output, nil
}
