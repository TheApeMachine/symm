package signal

import "slices"

// Median returns the median of values, or false when empty.
func Median(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}

	slices.Sort(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle], true
	}

	return (values[middle-1] + values[middle]) / 2, true
}
