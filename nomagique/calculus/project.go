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
func Project(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, hasX := input.Get(PortX)
	a, hasA := input.Get(PortA)
	b, hasB := input.Get(PortB)

	if !hasX || !hasA || !hasB || !utils.IsFinite(x) || !utils.IsFinite(a) || !utils.IsFinite(b) {
		return state, types.Frame{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: project requires finite x, a, and b",
			nil,
		))
	}

	denominator := b - a

	if denominator == 0 || !utils.IsFinite(denominator) {
		return state, types.Frame{}, fmt.Errorf("calculus: project requires distinct finite endpoints")
	}

	result := (x - a) / denominator

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: project overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}
