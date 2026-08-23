package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Extent emits the absolute distance between A and B.
*/
func Extent(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: extent requires finite a and b")
	}

	result := math.Abs(b - a)

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: extent overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Reflect mirrors X around the center of A and B: A+B-X.
func Reflect(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: reflect requires finite x, a, and b")
	}

	result := a + b - x

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: reflect overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Coherence emits central proximity within interval A..B, clamped to [0,1].
func Coherence(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: coherence requires finite x, a, and b")
	}

	extent := math.Abs(b - a)
	result := 1.0

	if extent > 0 {
		center := a/2 + b/2
		result = 1 - math.Abs(x-center)/(extent/2)
		result = math.Max(0, math.Min(1, result))
	}

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: coherence overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}
