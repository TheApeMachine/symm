package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Positive projects finite X onto the non-negative half-line.
*/
func Positive(input types.Frame) types.Frame {
	x, found := input.Get(PortX)

	if !found || !utils.IsFinite(x) {
		input.Err = fmt.Errorf("calculus: positive requires finite x")

		return input
	}

	input.Put(PortResult, math.Max(0, x))

	return input
}
