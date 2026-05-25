package stats

import "math"

/*
MedianAbsolute returns the median of |values|.
*/
func MedianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	magnitudes := make([]float64, len(values))

	for index, value := range values {
		magnitudes[index] = math.Abs(value)
	}

	return PercentileSorted(CopySorted(magnitudes), 0.5)
}
