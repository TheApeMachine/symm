package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Reflect mirrors X around the center of A and B: A+B-X.
*/
func Reflect(input types.Frame) types.Frame {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: reflect requires finite x, a, and b")

		return input
	}

	result := a + b - x

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: reflect overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
