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
func Squash(input *types.Frame) {
	value, hasX := input.Get(PortX)
	scale, hasScale := input.Get(SymbolScale)

	if !hasX || !hasScale || !utils.IsFinite(value) || !utils.IsFinite(scale) {
		input.Err = fmt.Errorf("calculus: squash requires finite value and scale")

		return
	}

	// Normalize both magnitudes by their common maximum before summing so the
	// denominator stays in [1,2] and never overflows for large finite inputs,
	// while (value/max)/denominator still equals value/(|scale|+|value|).
	result := 0.0
	maximum := math.Max(math.Abs(scale), math.Abs(value))
	denominator := math.Abs(scale)/maximum + math.Abs(value)/maximum

	if denominator > 0 {
		result = (value / maximum) / denominator
	}

	input.Put(PortResult, result)
}
