package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Maximum emits the greater of finite A and B.
*/
func Maximum(input *types.Frame) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		input.Err = fmt.Errorf("calculus: maximum requires a and b")

		return
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: maximum requires finite operands")

		return
	}

	result := a

	if b > a {
		result = b
	}

	input.Put(PortResult, result)
}
