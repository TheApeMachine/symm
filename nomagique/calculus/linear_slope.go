package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// EulerSumOfSquaresDenominator = 12.0 from Euler's summation formula:
// sum_{i=0}^{n-1} (i - (n-1)/2)^2 = n * (n^2 - 1) / 12.
const EulerSumOfSquaresDenominator = 12.0

// MinimumLinearRegressionSampleSize is the minimum number of observations required for a linear slope (n >= 2).
const MinimumLinearRegressionSampleSize = 2

/*
LinearSlope computes the ordinary least squares linear regression slope
of values against uniform step indices i = 0..n-1:
slope = (12 * sum((i - (n-1)/2) * y_i)) / (n * (n^2 - 1)).
Derived from exact Euler identity without magic constants.
*/
var LinearSlope types.Reduction = func(values []types.Number) types.Number {
	count := len(values)

	if count < MinimumLinearRegressionSampleSize {
		return 0
	}

	n := float64(count)
	xMean := (n - 1.0) / 2.0
	var numerator float64

	for index, value := range values {
		numerator += (float64(index) - xMean) * float64(value)
	}

	denominator := n * (n*n - 1.0) / EulerSumOfSquaresDenominator

	if denominator == 0 {
		return 0
	}

	return types.Number(numerator / denominator)
}
