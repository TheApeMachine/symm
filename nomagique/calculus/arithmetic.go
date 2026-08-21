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
Quotient divides a finite numerator by a strictly positive denominator. The
numerator remains signed, which distinguishes it from evidence-only Ratio.
*/
func Quotient(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	numerator, err := number(&input, SymbolLeft, "quotient")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	denominator, err := number(&input, SymbolRight, "quotient")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	if denominator <= 0 {
		return state, nomagique.Frame{}, &calculusError{
			primitive: "quotient",
			message:   "denominator must be positive",
		}
	}

	return state, resultFrame(input, numerator/denominator), nil
}

/*
Average returns the arithmetic center of two finite operands.
*/
func Average(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	left, err := number(&input, SymbolLeft, "average")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	right, err := number(&input, SymbolRight, "average")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, left+(right-left)/2), nil
}

/*
Absolute projects a finite scalar onto its magnitude.
*/
func Absolute(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "absolute")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, math.Abs(value)), nil
}

/*
Negative reflects a finite scalar through zero.
*/
func Negative(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "negative")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, -value), nil
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
