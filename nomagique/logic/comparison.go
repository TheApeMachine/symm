package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// GreaterThan emits one when A is strictly greater than B.
func GreaterThan(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: greater than requires finite a and b")
	}

	condition := 0.0

	if a > b {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// GreaterOrEqual emits one when A is greater than or equal to B.
func GreaterOrEqual(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: greater or equal requires finite a and b")
	}

	condition := 0.0

	if a >= b {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// LessThan emits one when A is strictly less than B.
func LessThan(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: less than requires finite a and b")
	}

	condition := 0.0

	if a < b {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// Equal emits one when A and B are exactly equal.
func Equal(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("logic: equal requires finite a and b")
	}

	condition := 0.0

	if a == b {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}

// PositiveOrder validates two configured facts as 0 < lower < upper.
func PositiveOrder(lower nomagique.Symbol, upper nomagique.Symbol) nomagique.Primitive {
	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		lowerValue, hasLower := input.Get(lower)
		upperValue, hasUpper := input.Get(upper)

		if !hasLower || !hasUpper || !utils.IsFinite(lowerValue) ||
			!utils.IsFinite(upperValue) || lowerValue <= 0 || upperValue <= lowerValue {
			return state, types.Frame{}, fmt.Errorf("logic: positive order requires 0 < lower < upper")
		}

		return state, input, nil
	}
}
