package calculus

import (
	"fmt"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Center emits the arithmetic center of A and B.
*/
func Center(input *types.Frame) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: center requires finite a and b",
			nil,
		))

		return
	}

	result := a/2 + b/2

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: center overflowed")

		return
	}

	input.Put(PortResult, result)
}
