package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Negative reflects finite X through zero.
*/
func Negative(input types.Frame) types.Frame {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		input.Err = fmt.Errorf("calculus: negative requires finite x")

		return input
	}

	result := -x

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: negative overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
