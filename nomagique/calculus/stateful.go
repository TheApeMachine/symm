package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// Accumulate adds the explicit input delta to the total retained in state.
func Accumulate(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	delta, found := input.Get(SymbolDelta)

	if !found || !utils.IsFinite(delta) {
		return state, types.Frame{}, fmt.Errorf("calculus: accumulate requires a finite delta")
	}

	total, hasTotal := state.Get(SymbolTotal)

	if hasTotal && !utils.IsFinite(total) {
		return state, types.Frame{}, fmt.Errorf("calculus: accumulate state must be finite")
	}

	result := total + delta

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: accumulate overflowed")
	}

	nextState := state
	nextState.Put(SymbolTotal, result)
	output := input
	output.Put(SymbolTotal, result)
	output.Put(PortResult, result)

	return nextState, output, nil
}

// Clear removes configured coordinates from committed state.
func Clear(symbols ...nomagique.Symbol) nomagique.Primitive {
	configured := append([]nomagique.Symbol(nil), symbols...)

	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		nextState := state

		for _, symbol := range configured {
			nextState.Delete(symbol)
		}

		return nextState, input, nil
	}
}

/*
Decay walks a retained level toward zero. Input level replaces state when
present. Clock supplies linear elapsed progress; shape, when present, is the
explicit remaining multiplier.
*/
func Decay(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	level, hasLevel := input.Get(SymbolLevel)

	if !hasLevel {
		level, hasLevel = state.Get(SymbolLevel)
	}

	if !hasLevel || !utils.IsFinite(level) {
		return state, types.Frame{}, fmt.Errorf("calculus: decay requires a finite level")
	}

	remaining := 0.0

	if clock, hasClock := input.Get(SymbolClock); hasClock {
		if !utils.IsFinite(clock) {
			return state, types.Frame{}, fmt.Errorf("calculus: decay clock must be finite")
		}

		remaining = 1 - clock

		if remaining < 0 {
			remaining = 0
		}
	}

	if shape, hasShape := input.Get(SymbolShape); hasShape {
		if !utils.IsFinite(shape) {
			return state, types.Frame{}, fmt.Errorf("calculus: decay shape must be finite")
		}

		remaining = shape
	}

	result := level * remaining

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: decay overflowed")
	}

	nextState := state
	nextState.Put(SymbolLevel, result)
	output := input
	output.Put(SymbolLevel, result)
	output.Put(PortResult, result)

	return nextState, output, nil
}
