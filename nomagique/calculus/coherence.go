package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Coherence emits central proximity within interval A..B, clamped to [0,1].
*/
func Coherence(input types.Frame) types.Frame {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: coherence requires finite x, a, and b",
			nil,
		))

		return input
	}

	extent := math.Abs(b - a)
	result := 1.0

	if extent > 0 {
		center := a/2 + b/2
		result = 1 - math.Abs(x-center)/(extent/2)
		result = math.Max(0, math.Min(1, result))
	}

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: coherence overflowed")

		return input
	}

	input.Put(PortResult, result)

	return input
}
