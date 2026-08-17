package learning

import "math"

func absExact(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
