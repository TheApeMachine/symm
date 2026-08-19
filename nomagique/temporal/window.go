package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCapacity = nomagique.MustIntern("capacity")
	SymbolUnixSec  = nomagique.MustIntern("unix_sec")
	SymbolUnixNsec = nomagique.MustIntern("unix_nsec")
)

/*
Window retains one bounded ring of values entirely inside its Frame state,
using the engine's generic sample slots. Retention is temporal plumbing: the
window keeps whatever the feeder observes, in arrival order, and every
estimator downstream reads the same slots.

Capacity is not a market magic number. It starts at one slot and doubles only
when the next observation would overflow the current ring, so a series earns
more memory by continuing to arrive — the count of observations is the only
governor. A caller may still seed an explicit capacity when it genuinely knows
one, but the default path stays data-driven and bounded by the engine's
fixed-sample ceiling.
*/
func Window(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, hasValue := input.Get(nomagique.SampleValue)
	_, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasValue || !hasSec || !hasNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window requires a value and event time",
		)
	}

	if nsec < 0 || nsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window requires normalized nanoseconds",
		)
	}

	capacity := windowCapacity(state, input)

	if capacity <= 0 || capacity > nomagique.MaxSamples {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window capacity must be an integer from 1 through %d",
			nomagique.MaxSamples,
		)
	}

	count := windowCount(state)
	head := windowHead(state)

	if count < 0 || count > capacity || head < 0 || head >= capacity {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window retained state is invalid for capacity %d",
			capacity,
		)
	}

	nextState := state
	slot := count

	if count >= capacity {
		slot = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	nextState.Put(nomagique.MustSampleSymbol(slot), value)
	nextState.Put(nomagique.SampleCount, float64(count))
	nextState.Put(nomagique.SampleHead, float64(head))
	nextState.Put(nomagique.SampleReady, 1)
	nextState.Put(SymbolCapacity, float64(capacity))

	output := input
	output.Put(nomagique.SampleCount, float64(count))
	output.Put(nomagique.SampleHead, float64(head))
	output.Put(nomagique.SampleReady, 1)
	output.Put(SymbolCapacity, float64(capacity))

	return nextState, output, nil
}

/*
windowCapacity resolves how many slots the ring may retain. The Span control
channel wins when present: the baseline emits the target size, and the window
slides, grows, or shrinks to it. Until a Span has been composed in, the window
bootstraps one slot and doubles only when the count reaches the current
capacity, bounded by the engine's sample ceiling.
*/
func windowCapacity(state nomagique.Frame, input nomagique.Frame) int {
	if capacityValue, found := input.Get(nmtypes.Span); found &&
		capacityValue > 0 &&
		capacityValue == math.Trunc(capacityValue) &&
		capacityValue <= nomagique.MaxSamples {
		return int(capacityValue)
	}

	current := windowCount(state)

	if current < 1 {
		return 1
	}

	next := current * 2

	if next > nomagique.MaxSamples {
		return nomagique.MaxSamples
	}

	return next
}

func windowCount(state nomagique.Frame) int {
	value, found := state.Get(nomagique.SampleCount)

	if !found {
		return 0
	}

	return int(value)
}

func windowHead(state nomagique.Frame) int {
	value, found := state.Get(nomagique.SampleHead)

	if !found {
		return 0
	}

	return int(value)
}
