package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Absolute projects finite X onto its magnitude.
*/
func Absolute(input types.Frame) types.Frame {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		input.Err = fmt.Errorf("calculus: absolute requires finite x")

		return input
	}

	input.Put(PortResult, math.Abs(x))

	return input
}
