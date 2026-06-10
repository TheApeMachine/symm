package numeric

import "math"

/*
InferTickSizeFromPrices estimates the exchange price increment from sorted adjacent prices.
*/
func InferTickSizeFromPrices(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	minDiff := math.Inf(1)

	for index := 1; index < len(prices); index++ {
		diff := math.Abs(prices[index] - prices[index-1])

		if diff <= 0 || math.IsNaN(diff) || math.IsInf(diff, 0) {
			continue
		}

		if diff < minDiff {
			minDiff = diff
		}
	}

	if math.IsInf(minDiff, 1) {
		return 0
	}

	return minDiff
}
