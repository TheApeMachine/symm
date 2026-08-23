package transport

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolCapacity = types.MustIntern("capacity")

/*
Window retains a bounded ring of scalar samples entirely inside its Frame state.
*/
func Window(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	capacityValue, hasCapacity := input.Get(SymbolCapacity)
	sample, hasSample := input.Get(types.SampleValue)

	if !hasCapacity || !hasSample {
		return state, types.Frame{}, fmt.Errorf(
			"transport: window requires capacity and sample",
		)
	}

	if capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
		capacityValue > types.MaxSamples || !finite(capacityValue, sample) {
		return state, types.Frame{}, fmt.Errorf(
			"transport: window capacity must be an integer from 1 through %d and sample must be finite",
			types.MaxSamples,
		)
	}

	capacity := int(capacityValue)
	count := integer(state, types.SampleCount)
	head := integer(state, types.SampleHead)

	if count < 0 || count > capacity || head < 0 || head >= capacity {
		return state, types.Frame{}, fmt.Errorf(
			"transport: window retained state is invalid for capacity %d",
			capacity,
		)
	}

	slot := count

	if count >= capacity {
		slot = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	nextState := state
	nextState.Put(SymbolCapacity, capacityValue)
	nextState.Put(types.MustSampleSymbol(slot), sample)
	nextState.Put(types.SampleCount, float64(count))
	nextState.Put(types.SampleHead, float64(head))
	nextState.Put(types.SampleReady, 1)

	return nextState, nextState, nil
}

/*
Samples returns only populated generic sample slots.
*/
func Samples(state types.Frame) types.Frame {
	output := types.Frame{}

	for index := range types.MaxSamples {
		symbol := types.MustSampleSymbol(index)
		value, found := state.Get(symbol)

		if found {
			output.Put(symbol, value)
		}
	}

	return output
}

func integer(frame types.Frame, symbol types.Symbol) int {
	value, found := frame.Get(symbol)

	if !found {
		return 0
	}

	return int(value)
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
