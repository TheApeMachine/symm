package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Center emits the arithmetic center of A and B.
*/
func Center(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, fmt.Errorf("calculus: center requires finite a and b")
	}

	result := a/2 + b/2

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: center overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}
