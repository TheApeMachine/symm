package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
)

/*
Sum adds finite left and right operands.
*/
func Sum(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	left, err := number(&input, SymbolLeft, "sum")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	right, err := number(&input, SymbolRight, "sum")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, left+right), nil
}

/*
Difference subtracts right from left.
*/
func Difference(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	left, err := number(&input, SymbolLeft, "difference")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	right, err := number(&input, SymbolRight, "difference")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, left-right), nil
}

/*
Product multiplies finite left and right operands.
*/
func Product(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	left, err := number(&input, SymbolLeft, "product")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	right, err := number(&input, SymbolRight, "product")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, left*right), nil
}

/*
Positive projects a finite scalar onto the non-negative half-line.
*/
func Positive(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "positive")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, math.Max(0, value)), nil
}

/*
Attack applies a finite jump to a finite base level.
*/
func Attack(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	base, err := number(&input, SymbolBase, "attack")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	jump, err := number(&input, SymbolJump, "attack")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	result := base + jump
	output := resultFrame(input, result)
	output.Put(SymbolBase, result)

	return state, output, nil
}

/*
Rate calculates count divided by positive duration.
*/
func Rate(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	count, err := number(&input, SymbolCount, "rate")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	duration, err := number(&input, SymbolDuration, "rate")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	if duration <= 0 {
		return state, nomagique.Frame{}, rateError("duration must be positive")
	}

	output := input
	output.Put(SymbolRate, count/duration)

	return state, output, nil
}

func rateError(message string) error {
	return &calculusError{primitive: "rate", message: message}
}

type calculusError struct {
	primitive string
	message   string
}

func (calculusErr *calculusError) Error() string {
	return "calculus: " + calculusErr.primitive + " " + calculusErr.message
}
