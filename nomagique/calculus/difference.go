package calculus

import (
	"fmt"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Difference subtracts B from A.
*/
func Difference(input types.Frame) types.Frame {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: difference requires a and b",
			nil,
		))

		return input
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: difference requires finite operands",
			nil,
		))

		return input
	}

	result := a - b

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: difference overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
