package engine

import "math"

/*
AlignConfidence combines dimensionless indicator strengths from the current
measurement into (0, 1). Only indicators present in this reading contribute;
absent hallmarks reduce alignment but do not zero it unless nothing fired.
*/
func AlignConfidence(factors ...float64) float64 {
	product := 1.0
	count := 0

	for _, factor := range factors {
		if factor <= 0 {
			continue
		}

		product *= factor
		count++
	}

	if count == 0 {
		return 0
	}

	mean := math.Pow(product, 1.0/float64(count))

	return mean / (mean + 1)
}

/*
ConfidenceFromScore maps one bounded measurement in (0, 1] into display strength.
Used when the score itself is the reading, not one of several hallmarks.
*/
func ConfidenceFromScore(score float64) float64 {
	if score <= 0 {
		return 0
	}

	return 1 - math.Exp(-score)
}

/*
ExcessRatio maps values above unity into (0, 1): how far past the unit threshold.
*/
func ExcessRatio(value float64) float64 {
	if value <= 1 {
		return 0
	}

	return (value - 1) / value
}
