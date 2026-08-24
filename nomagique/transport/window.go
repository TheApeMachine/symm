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
func Window(input types.Frame) types.Frame {
	capacityValue, hasCapacity := input.Get(SymbolCapacity)
	sample, hasSample := input.Get(types.SampleValue)

	if !hasCapacity || !hasSample {
		input.Err = fmt.Errorf(
			"transport: window requires capacity and sample",
		)

		return input
	}

	if capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
		capacityValue > types.MaxSamples || !finite(capacityValue, sample) {
		input.Err = fmt.Errorf(
			"transport: window capacity must be an integer from 1 through %d and sample must be finite",
			types.MaxSamples,
		)

		return input
	}

	capacity := int(capacityValue)
	count := integer(input, types.SampleCount)
	head := integer(input, types.SampleHead)

	if count < 0 || count > capacity || head < 0 || head >= capacity {
		input.Err = fmt.Errorf(
			"transport: window retained state is invalid for capacity %d",
			capacity,
		)

		return input
	}

	slot := count

	if count >= capacity {
		slot = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	input.Put(SymbolCapacity, capacityValue)
	input.Put(types.MustSampleSymbol(slot), sample)
	input.Put(types.SampleCount, float64(count))
	input.Put(types.SampleHead, float64(head))
	input.Put(types.SampleReady, 1)

	return input
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
