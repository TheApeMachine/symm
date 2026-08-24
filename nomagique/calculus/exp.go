package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Exp projects the exponential e^x of a finite X.
*/
func Exp(input types.Frame) types.Frame {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		input.Err = fmt.Errorf("calculus: exp requires a finite x")

		return input
	}

	input.Put(PortResult, math.Exp(x))

	return input
}
