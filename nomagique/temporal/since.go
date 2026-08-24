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
func Since(input types.Frame) types.Frame {
	seconds, hasSeconds := input.Get(SymbolUnixSec)
	nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

	if !hasSeconds || !hasNanoseconds || nanoseconds < 0 || nanoseconds >= 1e9 {
		input.Err = fmt.Errorf(
			"temporal: since requires normalized event-time coordinates",
		)

		return input
	}

	originSeconds, hasOriginSeconds := input.Get(SymbolOriginSec)
	originNanoseconds, hasOriginNanoseconds := input.Get(SymbolOriginNsec)

	if !hasOriginSeconds || !hasOriginNanoseconds {
		originSeconds = seconds
		originNanoseconds = nanoseconds
		input.Put(SymbolOriginSec, originSeconds)
		input.Put(SymbolOriginNsec, originNanoseconds)
	}

	duration := seconds - originSeconds +
		(nanoseconds-originNanoseconds)*1e-9

	if duration < 0 {
		input.Err = fmt.Errorf(
			"temporal: since event time must not precede its origin",
		)

		return input
	}

	advanced := 0.0

	if duration > 0 {
		advanced = 1
	}

	input.Put(calculus.SymbolDuration, duration)
	input.Put(SymbolObservedSec, originSeconds)
	input.Put(SymbolObservedNsec, originNanoseconds)
	input.Put(SymbolAdvanced, advanced)
	completed, _ := input.Get(SymbolCompletedSpans)
	input.Put(SymbolCompletedSpans, completed)

	return input
}

/*
Restart moves the retained origin to the current event and counts one completed
span. It preserves the output produced for the span that just ended.
*/
func Restart(input types.Frame) types.Frame {
	seconds, hasSeconds := input.Get(SymbolUnixSec)
	nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

	if !hasSeconds || !hasNanoseconds || nanoseconds < 0 || nanoseconds >= 1e9 {
		input.Err = fmt.Errorf(
			"temporal: restart requires normalized event-time coordinates",
		)

		return input
	}

	completed, _ := input.Get(SymbolCompletedSpans)
	input.Put(SymbolOriginSec, seconds)
	input.Put(SymbolOriginNsec, nanoseconds)
	input.Put(SymbolCompletedSpans, completed+1)

	return input
}
