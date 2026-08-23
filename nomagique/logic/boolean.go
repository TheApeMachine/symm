package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// And emits one when both finite operands are non-zero.
func And(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: and requires finite a and b")
	}

	condition := 0.0

	if a != 0 && b != 0 {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// Or emits one when either finite operand is non-zero.
func Or(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: or requires finite a and b")
	}

	condition := 0.0

	if a != 0 || b != 0 {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// Xor emits one when exactly one finite operand is non-zero.
func Xor(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: xor requires finite a and b")
	}

	condition := 0.0

	if (a != 0) != (b != 0) {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// Not inverts one finite numeric condition.
func Not(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	condition, found := input.Get(SymbolCondition)

	if !found || !utils.IsFinite(condition) {
		return state, types.Frame{}, fmt.Errorf("logic: not requires a finite condition")
	}

	result := 0.0

	if condition == 0 {
		result = 1
	}

	output := input
	output.Put(SymbolCondition, result)
	output.Put(SymbolResult, result)

	return state, output, nil
}
