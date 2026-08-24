package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Squash maps X through a non-negative scale as X/(scale+|X|). It preserves sign,
is bounded by [-1,1], and emits zero only for the degenerate zero scale/value.
A negative scale is treated as its absolute value so signed baselines do not
poison the bound.
*/
func Squash(input types.Frame) types.Frame {
	x, hasX := input.Get(PortX)
	scale, hasScale := input.Get(SymbolScale)

	if !hasX || !hasScale || !utils.IsFinite(x) || !utils.IsFinite(scale) {
		input.Err = fmt.Errorf("calculus: squash requires finite x and scale")

		return input
	}

	result := 0.0
	denominator := math.Abs(scale) + math.Abs(x)

	if denominator > 0 {
		result = x / denominator
	}

	input.Put(PortResult, result)

	return input
}
