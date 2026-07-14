package types

import "math"

/*
NormalizeFinite retains one finite scalar for graph comparison. Non-finite and
exact-zero values stay explicit nils rather than being coerced.
*/
func NormalizeFinite(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 {
		return nil
	}

	normalized := value

	return &normalized
}

/*
NormalizeRatio divides value by baseline when the baseline is positive.
*/
func NormalizeRatio(value float64, baseline float64) *float64 {
	if baseline <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	normalized := value / baseline

	if math.IsNaN(normalized) || math.IsInf(normalized, 0) || normalized == 0 {
		return nil
	}

	return &normalized
}

/*
NormalizeDeviation reports excess over baseline relative to baseline magnitude.
*/
func NormalizeDeviation(value float64, baseline float64) *float64 {
	if baseline <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	normalized := (value - baseline) / baseline

	if math.IsNaN(normalized) || math.IsInf(normalized, 0) || normalized == 0 {
		return nil
	}

	return &normalized
}

/*
NormalizeSigned retains directional finite values, including negative evidence.
*/
func NormalizeSigned(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 {
		return nil
	}

	normalized := value

	return &normalized
}
