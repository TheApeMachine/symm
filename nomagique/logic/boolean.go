package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

/*
And emits true when both living operands are non-zero.
*/
func And(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return boolean(state, input, "and", func(left float64, right float64) bool {
		return left != 0 && right != 0
	})
}

/*
Or emits true when either living operand is non-zero.
*/
func Or(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return boolean(state, input, "or", func(left float64, right float64) bool {
		return left != 0 || right != 0
	})
}

/*
Xor emits true when exactly one living operand is non-zero.
*/
func Xor(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return boolean(state, input, "xor", func(left float64, right float64) bool {
		return (left != 0) != (right != 0)
	})
}

/*
Not inverts the condition slot.
*/
func Not(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	condition, found := input.Get(SymbolCondition)

	if !found {
		return state, nomagique.Frame{}, fmt.Errorf(
			"logic: not requires a condition",
		)
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

func boolean(
	state nomagique.Frame,
	input nomagique.Frame,
	name string,
	predicate func(float64, float64) bool,
) (nomagique.Frame, nomagique.Frame, error) {
	left, hasLeft := input.Get(calculus.SymbolLeft)
	right, hasRight := input.Get(calculus.SymbolRight)

	if !hasLeft || !hasRight {
		return state, nomagique.Frame{}, fmt.Errorf(
			"logic: %s requires left and right operands",
			name,
		)
	}

	condition := 0.0

	if predicate(left, right) {
		condition = 1
	}

	output := input
	output.Put(SymbolCondition, condition)
	output.Put(SymbolResult, condition)

	return state, output, nil
}
