package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// LogRatio computes log(current/previous) for positive finite observations.
func LogRatio(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	current, hasCurrent := input.Get(SymbolCurrent)
	previous, hasPrevious := input.Get(SymbolPrevious)

	if !hasCurrent || !hasPrevious || !utils.IsFinite(current) ||
		!utils.IsFinite(previous) {
		return state, types.Frame{}, fmt.Errorf("calculus: log ratio requires finite current and previous")
	}

	if current <= 0 || previous <= 0 {
		return state, types.Frame{}, fmt.Errorf("calculus: log ratio requires positive operands")
	}

	result := math.Log(current / previous)

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: log ratio produced a non-finite result")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

/*
Squash maps X through a non-negative scale as X/(scale+|X|). It preserves sign,
is bounded by [-1,1], and emits zero only for the degenerate zero scale/value.
A negative scale is treated as its absolute value so signed baselines do not
poison the bound.
*/
func Squash(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	x, hasX := input.Get(PortX)
	scale, hasScale := input.Get(SymbolScale)

	if !hasX || !hasScale || !utils.IsFinite(x) || !utils.IsFinite(scale) {
		return state, types.Frame{}, fmt.Errorf("calculus: squash requires finite x and scale")
	}

	result := 0.0
	denominator := math.Abs(scale) + math.Abs(x)

	if denominator > 0 {
		result = x / denominator
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

/*
Inverse is the compatibility reciprocal evidence map scale/(scale+value).
New interval reflection should use Reflect.

Deprecated: use a deliberately wired affine or reciprocal composition.
*/
func Inverse(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	value, hasValue := input.Get(PortX)
	scale, hasScale := input.Get(SymbolScale)

	if !hasValue || !hasScale || !utils.IsFinite(value) || !utils.IsFinite(scale) {
		return state, types.Frame{}, fmt.Errorf("calculus: inverse requires finite value and scale")
	}

	if scale < 0 {
		return state, types.Frame{}, fmt.Errorf("calculus: inverse scale must not be negative")
	}

	result := 0.0

	switch {
	case value < 0:
		result = 0
	case value == 0:
		result = 1
	case scale > 0:
		result = scale / (scale + value)
	}

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: inverse produced a non-finite result")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

/*
Ratio is the compatibility readiness-gated normalization value/baseline.
New equations should compose Quotient with an explicit logic gate.

Deprecated: wire Quotient behind an explicit logic route.
*/
func Ratio(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	value, hasValue := input.Get(PortX)
	baseline, hasBaseline := input.Get(SymbolBaseline)
	ready, hasReady := input.Get(SymbolReady)

	if !hasValue || !hasBaseline || !hasReady || !utils.IsFinite(value) ||
		!utils.IsFinite(baseline) || !utils.IsFinite(ready) {
		return state, types.Frame{}, fmt.Errorf("calculus: ratio requires finite value, baseline, and ready")
	}

	result := 0.0

	if ready != 0 && value > 0 && baseline > 0 {
		result = value / baseline
	}

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: ratio produced a non-finite result")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}

// Exponential computes e raised to negative finite progress.
func Exponential(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	progress, found := input.Get(SymbolProgress)

	if !found || !utils.IsFinite(progress) {
		return state, types.Frame{}, fmt.Errorf("calculus: exponential requires finite progress")
	}

	result := math.Exp(-progress)

	if !utils.IsFinite(result) {
		return state, types.Frame{}, fmt.Errorf("calculus: exponential overflowed")
	}

	output := input
	output.Put(PortResult, result)

	return state, output, nil
}
