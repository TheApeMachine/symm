package utils

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
NormalizeRatio divides value by baseline when the baseline is positive.
*/
func NormalizeRatio(value float64, baseline float64) float64 {
	if !ValidatePositive(baseline) {
		return value
	}

	normalized := value / baseline

	if !ValidateNonZero(normalized) {
		return value
	}

	return normalized
}

/*
NormalizeDeviation reports excess over baseline relative to baseline magnitude.
*/
func NormalizeDeviation(value float64, baseline float64) float64 {
	if !ValidatePositive(baseline) {
		return value
	}

	normalized := (value - baseline) / baseline

	if !ValidateNonZero(normalized) {
		return value
	}

	return normalized
}

/*
ValidateFinite checks if a value is finite and not NaN.
*/
func ValidateFinite(value float64) bool {
	// IEEE 754 check: NaN and +-Inf fail this comparison.
	if math.Abs(value) <= math.MaxFloat64 {
		return true
	}

	logError("value must be finite")
	return false
}

func ValidatePositive(value float64) bool {
	// Checks positive and finite in 2 float comparison operations.
	if value > 0 && value <= math.MaxFloat64 {
		return true
	}

	logError("value must be positive and finite")
	return false
}

func ValidateNonZero(value float64) bool {
	// Checks non-zero and finite directly without call chain overhead.
	if value != 0 && math.Abs(value) <= math.MaxFloat64 {
		return true
	}

	logError("value is zero or not finite")
	return false
}

// Cold path logging functions. Separating these ensures the hot validation logic
// stays below Go's compiler inlining threshold (80 AST nodes).

//go:noinline
func logError(msg string) {
	errnie.Error(errnie.Err(
		errnie.Validation,
		msg,
		nil,
	))
}
