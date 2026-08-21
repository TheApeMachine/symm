package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

/*
GreaterThan emits true when the left living value exceeds the right living
value.
*/
func GreaterThan(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return compare(state, input, "greater than", func(left float64, right float64) bool {
		return left > right
	})
}

/*
LessThan emits true when the left living value is below the right living value.
*/
func LessThan(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return compare(state, input, "less than", func(left float64, right float64) bool {
		return left < right
	})
}

/*
Equal emits true when the two living values are exactly equal.
*/
func Equal(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return compare(state, input, "equal", func(left float64, right float64) bool {
		return left == right
	})
}

func compare(
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
