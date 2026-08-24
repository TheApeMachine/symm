package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Average returns the overflow-resistant arithmetic center of A and B.
*/
func Average(input types.Frame) types.Frame {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		input.Err = fmt.Errorf("calculus: average requires a and b")

		return input
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: average requires finite operands")

		return input
	}

	result := a/2 + b/2

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: average overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
