package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
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
estimator downstream reads the same slots. The ring is the widest memory a
composition has; sharpness is never the window's to decide.
*/
func Window(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	capacityValue, hasCapacity := input.Get(SymbolCapacity)
	value, hasValue := input.Get(nomagique.SampleValue)

	if !hasCapacity || !hasValue {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window requires capacity and a value",
		)
	}

	if capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
		capacityValue > nomagique.MaxSamples {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: window capacity must be an integer from 1 through %d",
			nomagique.MaxSamples,
		)
	}

	capacity := int(capacityValue)
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
	nextState.Put(SymbolCapacity, capacityValue)

	output := input
	output.Put(nomagique.SampleCount, float64(count))
	output.Put(nomagique.SampleHead, float64(head))
	output.Put(nomagique.SampleReady, 1)

	return nextState, output, nil
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
