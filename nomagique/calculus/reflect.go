package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Reflect mirrors X around the center of A and B: A+B-X.
*/
func Reflect(input *types.Frame) {
	value, hasValue := input.Get(PortX)
	axisA, hasAxisA := input.Get(PortA)
	axisB, hasAxisB := input.Get(PortB)

	if !hasValue || !hasAxisA || !hasAxisB || !utils.IsFinite(value) ||
		!utils.IsFinite(axisA) || !utils.IsFinite(axisB) {
		input.Err = fmt.Errorf("calculus: reflect requires finite x, a, and b")

		return
	}

	result := axisA + axisB - value

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: reflect overflowed")

		return
	}

	input.Put(PortResult, result)
}
