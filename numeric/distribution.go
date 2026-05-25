package numeric

import (
	"math"
	"sort"
)

/*
CopySorted returns a sorted copy of values.
*/
func CopySorted(values []float64) []float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return sorted
}

/*
Median returns the median of values.
*/
func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := CopySorted(values)
	mid := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[mid]
	}

	return (sorted[mid-1] + sorted[mid]) / 2
}

/*
Mean returns the arithmetic mean of values.
*/
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

/*
Max returns the largest value.
*/
func Max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	maxValue := values[0]

	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}

	return maxValue
}

/*
PercentileSorted reads a sorted slice at quantile p in [0, 1].
*/
func PercentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	if lower == upper {
		return sorted[lower]
	}

	weight := position - float64(lower)

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

/*
Quartiles returns the lower and upper quartiles of values.
*/
func Quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sorted := CopySorted(values)
	n := len(sorted)
	lower = sorted[n/4]
	upper = sorted[(3*n)/4]

	return lower, upper
}

/*
MedianAbsoluteDeviation returns MAD around median on a sorted slice.
*/
func MedianAbsoluteDeviation(sorted []float64, median float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	deviations := make([]float64, len(sorted))

	for index, value := range sorted {
		delta := value - median

		if delta < 0 {
			delta = -delta
		}

		deviations[index] = delta
	}

	return Median(deviations)
}

/*
MedianAbsolute returns the median absolute value.
*/
func MedianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	absolute := make([]float64, len(values))

	for index, value := range values {
		if value < 0 {
			value = -value
		}

		absolute[index] = value
	}

	return Median(absolute)
}
