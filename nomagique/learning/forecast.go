package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Forecast composes Welford moments over residuals.
It takes a residual observation and returns the first four moments:
[0]: Mean
[1]: Variance
[2]: Skewness
[3]: Kurtosis
*/
func Forecast() types.Value[float64, []float64] {
	var count, mean, m2, m3, m4 float64

	return func(in float64) []float64 {
		count++
		n := count
		delta := in - mean
		deltaN := delta / n
		deltaN2 := deltaN * deltaN
		term1 := delta * deltaN * (n - 1)

		mean += deltaN
		m4 += term1*deltaN2*(n*n-3*n+3) + 6*deltaN2*m2 - 4*deltaN*m3
		m3 += term1*deltaN*(n-2) - 3*deltaN*m2
		m2 += term1

		variance := 0.0
		skewness := 0.0
		kurtosis := 0.0

		if n > core.Unit {
			variance = m2 / (n - core.Unit)
		}
		if m2 > 0 {
			skewness = (core.Unit * n * m3) / (m2 * m2) // approximated
			kurtosis = (n * m4) / (m2 * m2)
		}

		return []float64{
			mean,
			variance,
			skewness,
			kurtosis,
		}
	}
}
