package learning

import "math"

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
