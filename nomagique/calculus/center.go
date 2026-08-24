package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Center emits the arithmetic center of A and B.
*/
func Center(input types.Frame) types.Frame {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: center requires finite a and b")

		return input
	}

	result := a/2 + b/2

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: center overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
