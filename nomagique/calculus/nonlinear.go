package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

/*
LogRatio computes log(current / previous) for positive finite observations.
*/
func LogRatio(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	current, err := number(&input, SymbolCurrent, "log ratio")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	previous, err := number(&input, SymbolPrevious, "log ratio")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	if current <= 0 || previous <= 0 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"calculus: log ratio requires positive operands",
		)
	}

	return state, resultFrame(input, math.Log(current/previous)), nil
}

/*
Squash maps positive evidence through a positive empirical scale.
*/
func Squash(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "squash")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	scale, err := number(&input, SymbolScale, "squash")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	result := 0.0

	if value > 0 && scale > 0 {
		result = value / (scale + value)
	}

	return state, resultFrame(input, result), nil
}

/*
Inverse maps counter-evidence through an empirical scale. Measured absence maps
to one without requiring a positive scale.
*/
func Inverse(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "inverse")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	scale, err := number(&input, SymbolScale, "inverse")

	if err != nil {
		return state, nomagique.Frame{}, err
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

	return state, resultFrame(input, result), nil
}

/*
Ratio normalizes positive evidence against a positive baseline when ready is
non-zero.
*/
func Ratio(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	value, err := number(&input, SymbolValue, "ratio")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	baseline, err := number(&input, SymbolBaseline, "ratio")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	ready, err := number(&input, SymbolReady, "ratio")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	result := 0.0

	if ready != 0 && value > 0 && baseline > 0 {
		result = value / baseline
	}

	return state, resultFrame(input, result), nil
}

/*
Exponential computes e raised to negative progress.
*/
func Exponential(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	progress, err := number(&input, SymbolProgress, "exponential")

	if err != nil {
		return state, nomagique.Frame{}, err
	}

	return state, resultFrame(input, math.Exp(-progress)), nil
}
