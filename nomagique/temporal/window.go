package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCapacity = nmtypes.MustIntern("capacity")
	SymbolUnixSec  = nmtypes.MustIntern("unix_sec")
	SymbolUnixNsec = nmtypes.MustIntern("unix_nsec")
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
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	value, hasValue := input.Get(nmtypes.SampleValue)
	_, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasValue || !hasSec || !hasNsec {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: window requires a value and event time",
		)
	}

	if nsec < 0 || nsec >= 1e9 {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: window requires normalized nanoseconds",
		)
	}

	capacity, err := windowCapacity(state, input)

	if err != nil {
		return state, types.Frame{}, err
	}

	if capacity <= 0 || capacity > nmtypes.MaxSamples {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: window capacity must be an integer from 1 through %d",
			nmtypes.MaxSamples,
		)
	}

	count := windowCount(state)
	head := windowHead(state)

	if count < 0 || count > capacity || head < 0 || head >= capacity {
		return state, types.Frame{}, fmt.Errorf(
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

	nextState.Put(nmtypes.MustSampleSymbol(slot), value)
	nextState.Put(nmtypes.SampleCount, float64(count))
	nextState.Put(nmtypes.SampleHead, float64(head))
	nextState.Put(nmtypes.SampleReady, 1)
	nextState.Put(SymbolCapacity, float64(capacity))

	output := nextState
	output.Merge(input)
	output.Put(nmtypes.SampleCount, float64(count))
	output.Put(nmtypes.SampleHead, float64(head))
	output.Put(nmtypes.SampleReady, 1)
	output.Put(SymbolCapacity, float64(capacity))

	return nextState, output, nil
}

/*
windowCapacity resolves how many slots the ring may retain. The Span control
channel wins when present, from the input first (a Configure-wired producer
emitted it this step) and then from state (a previous step's baseline verdict
that Configure merged into state). Until a Span has been composed in, the
window bootstraps one slot and doubles only when the count reaches the current
capacity, bounded by the engine's sample ceiling.
*/
func windowCapacity(state types.Frame, input types.Frame) (int, error) {
	var (
		capacityValue float64
		found         bool
	)

	if capacityValue, found = input.Get(nmtypes.Span); !found {
		capacityValue, found = state.Get(nmtypes.Span)
	}

	if found {
		if capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
			capacityValue > nmtypes.MaxSamples {
			return 0, fmt.Errorf(
				"temporal: span control channel must be an integer from 1 through %d",
				nmtypes.MaxSamples,
			)
		}

		return int(capacityValue), nil
	}

	current := windowCount(state)

	if current < 1 {
		return 1, nil
	}

	capacityValue, hasCapacity := state.Get(SymbolCapacity)

	if hasCapacity && current < int(capacityValue) {
		return int(capacityValue), nil
	}

	next := int(capacityValue + capacityValue)

	if next > nmtypes.MaxSamples {
		return nmtypes.MaxSamples, nil
	}

	return next, nil
}

func windowCount(state types.Frame) int {
	value, found := state.Get(nmtypes.SampleCount)

	if !found {
		return 0
	}

	return int(value)
}

func windowHead(state types.Frame) int {
	value, found := state.Get(nmtypes.SampleHead)

	if !found {
		return 0
	}

	return int(value)
}
