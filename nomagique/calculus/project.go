package calculus

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Project emits the affine coordinate (X-A)/(B-A).
*/
func Project(input *types.Frame) {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: project requires finite x, a, and b",
			nil,
		))

		return
	}

	denominator := b - a

	if denominator == 0 || !utils.IsFinite(denominator) {
		input.Err = fmt.Errorf("calculus: project requires distinct finite endpoints")

		return
	}

	result := (x - a) / denominator

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: project overflowed")

		return
	}

	input.Put(PortResult, result)
}
