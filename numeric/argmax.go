package numeric

import "math"

/*
ArgmaxStringFloat64 returns the key with maximal value. For an empty map it
returns ("", -1). Ties follow map iteration order (same as a bare range loop).
*/
func ArgmaxStringFloat64(values map[string]float64) (bestKey string, bestValue float64) {
	if len(values) == 0 {
		return "", -1
	}

	bestKey = ""
	bestValue = math.Inf(-1)

	for key, value := range values {
		if value > bestValue {
			bestValue = value
			bestKey = key
		}
	}

	return bestKey, bestValue
}
