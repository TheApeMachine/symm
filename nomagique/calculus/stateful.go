package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

/*
Accumulate adds an input delta to the total retained in state.
*/
func Accumulate(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	delta, found := input.Get(SymbolDelta)

	if !found {
		delta, found = input.Get(SymbolValue)
	}

	if !found || !finite(delta) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"calculus: accumulate requires a finite delta",
		)
	}

	total, hasTotal := state.Get(SymbolTotal)

	if hasTotal && !finite(total) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"calculus: accumulate state must be finite",
		)
	}

	total += delta
	nextState := state
	nextState.Put(SymbolTotal, total)
	output := input
	output.Put(SymbolTotal, total)
	output.Put(SymbolResult, total)

	return nextState, output, nil
}

/*
Clear removes configured coordinates from committed state while preserving the
current output. It is the reset atom used by bounded accumulation equations.
*/
func Clear(symbols ...nomagique.Symbol) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		nextState := state

		for _, symbol := range symbols {
			nextState.Delete(symbol)
		}

		return nextState, input, nil
	}
}

/*
Decay walks a retained level toward zero. Input level initializes or replaces
state; input clock controls linear progress, while optional shape overrides the
remaining fraction.
*/
func Decay(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	level, hasLevel := input.Get(SymbolLevel)

	if !hasLevel {
		level, hasLevel = state.Get(SymbolLevel)
	}

	if !hasLevel || !finite(level) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"calculus: decay requires a finite level",
		)
	}

	clock, hasClock := input.Get(SymbolClock)
	remaining := 0.0

	if hasClock {
		if !finite(clock) {
			return state, nomagique.Frame{}, fmt.Errorf(
				"calculus: decay clock must be finite",
			)
		}

		remaining = 1 - clock

		if remaining < 0 {
			remaining = 0
		}
	}

	if shape, hasShape := input.Get(SymbolShape); hasShape {
		if !finite(shape) {
			return state, nomagique.Frame{}, fmt.Errorf(
				"calculus: decay shape must be finite",
			)
		}

		remaining = shape
	}

	result := level * remaining
	nextState := state
	nextState.Put(SymbolLevel, result)
	output := input
	output.Put(SymbolLevel, result)
	output.Put(SymbolResult, result)

	return nextState, output, nil
}
