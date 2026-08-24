package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Product multiplies finite A and B operands.
*/
func Product(input types.Frame) types.Frame {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		input.Err = fmt.Errorf("calculus: product requires a and b")

		return input
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: product requires finite operands")

		return input
	}

	result := a * b

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: product overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
