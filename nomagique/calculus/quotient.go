package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Quotient divides finite A by finite, non-zero B.
*/
func Quotient(input *types.Frame) {
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasA || !hasB {
		input.Err = fmt.Errorf("calculus: quotient requires a and b")

		return
	}

	if !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("calculus: quotient requires finite operands")

		return
	}

	if b == 0 {
		input.Err = fmt.Errorf("calculus: quotient denominator must be non-zero")

		return
	}

	result := a / b

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: quotient overflowed")

		return
	}

	input.Put(PortResult, result)
}
