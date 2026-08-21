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
GreaterOrEqual emits true when the left living value is not below the right.
*/
func GreaterOrEqual(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	return compare(state, input, "greater or equal", func(left float64, right float64) bool {
		return left >= right
	})
}

/*
PositiveOrder verifies that two configured coordinates form a strictly
positive increasing interval. It is the reusable precondition for geometry
whose width is defined as upper minus lower.
*/
func PositiveOrder(lower nomagique.Symbol, upper nomagique.Symbol) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		lowerValue, hasLower := input.Get(lower)
		upperValue, hasUpper := input.Get(upper)

		if !hasLower || !hasUpper || lowerValue <= 0 || upperValue <= lowerValue {
			return state, nomagique.Frame{}, fmt.Errorf(
				"logic: positive order requires 0 < lower < upper",
			)
		}

		return state, input, nil
	}
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
