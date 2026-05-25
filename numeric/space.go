package numeric

import "math"

/*
LinSpace returns evenly spaced values from minValue through maxValue inclusive.
*/
func LinSpace(minValue, maxValue float64, steps int) []float64 {
	if steps <= 1 {
		return []float64{(minValue + maxValue) / 2}
	}

	values := make([]float64, steps)
	stepSize := (maxValue - minValue) / float64(steps-1)

	for index := range steps {
		values[index] = minValue + float64(index)*stepSize
	}

	return values
}

/*
LogSpace returns log-uniform values from minValue through maxValue inclusive.
*/
func LogSpace(minValue, maxValue float64, steps int) []float64 {
	if steps <= 1 {
		return []float64{math.Sqrt(minValue * maxValue)}
	}

	if minValue <= 0 || maxValue <= 0 {
		return []float64{maxValue}
	}

	if minValue > maxValue {
		minValue, maxValue = maxValue, minValue
	}

	logMin := math.Log(minValue)
	logMax := math.Log(maxValue)
	values := make([]float64, steps)
	stepSize := (logMax - logMin) / float64(steps-1)

	for index := range steps {
		values[index] = math.Exp(logMin + float64(index)*stepSize)
	}

	return values
}
