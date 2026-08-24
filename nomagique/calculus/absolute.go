package calculus

import (
	"math"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Absolute projects finite X onto its magnitude.
*/
func Absolute(input types.Frame) types.Frame {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: absolute requires finite x",
			nil,
		))

		return input
	}

	input.Put(PortResult, math.Abs(x))

	return input
}
