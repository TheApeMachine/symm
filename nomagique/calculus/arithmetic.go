package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// Sum adds finite A and B operands.
func Sum(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		return state, types.Frame{}, fmt.Errorf("calculus: sum requires a and b")
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: sum requires finite operands")
	}

	result := a + b

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: sum overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Difference subtracts B from A.
func Difference(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		return state, types.Frame{}, fmt.Errorf("calculus: difference requires a and b")
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: difference requires finite operands")
	}

	result := a - b

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: difference overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Product multiplies finite A and B operands.
func Product(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		return state, types.Frame{}, fmt.Errorf("calculus: product requires a and b")
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: product requires finite operands")
	}

	result := a * b

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: product overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Quotient divides finite A by finite, non-zero B.
func Quotient(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		return state, types.Frame{}, fmt.Errorf("calculus: quotient requires a and b")
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: quotient requires finite operands")
	}

	if b == 0 {
		return state, types.Frame{}, fmt.Errorf("calculus: quotient denominator must be non-zero")
	}

	result := a / b

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: quotient overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Average returns the overflow-resistant arithmetic center of A and B.
func Average(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		return state, types.Frame{}, fmt.Errorf("calculus: average requires a and b")
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: average requires finite operands")
	}

	result := a/2 + b/2

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: average overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Absolute projects finite X onto its magnitude.
func Absolute(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		return state, types.Frame{}, fmt.Errorf("calculus: absolute requires finite x")
	}

	output := input
	output.Put(PortResult, math.Abs(x))

	return state, output, nil
}

// Negative reflects finite X through zero.
func Negative(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		return state, types.Frame{}, fmt.Errorf("calculus: negative requires finite x")
	}

	result := -x

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: negative overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Positive projects finite X onto the non-negative half-line.
func Positive(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		return state, types.Frame{}, fmt.Errorf("calculus: positive requires finite x")
	}

	output := input
	output.Put(PortResult, math.Max(0, x))

	return state, output, nil
}

// Attack applies a finite jump to a finite base level.
func Attack(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	base, hasBase := input.Get(SymbolBase)
	jump, hasJump := input.Get(SymbolJump)

	if !hasBase || !hasJump || !utils.IsFinite(base) || !utils.IsFinite(jump) {
		return state, types.Frame{}, fmt.Errorf("calculus: attack requires finite base and jump")
	}

	result := base + jump

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: attack overflowed")
	}

	output := input
	output.Put(SymbolBase, result)
	output.Put(PortResult, result)

	return state, output, nil
}

// Rate calculates finite count divided by positive finite duration.
func Rate(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	count, hasCount := input.Get(SymbolCount)
	duration, hasDuration := input.Get(SymbolDuration)

	if !hasCount || !hasDuration || !utils.IsFinite(count) || !utils.IsFinite(duration) {
		return state, types.Frame{}, fmt.Errorf("calculus: rate requires finite count and duration")
	}

	if duration <= 0 {
		return state, types.Frame{}, fmt.Errorf("calculus: rate duration must be positive")
	}

	result := count / duration

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: rate overflowed")
	}

	output := input
	output.Put(SymbolRate, result)

	return state, output, nil
}
