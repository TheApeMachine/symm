package correlation

import (
	"math"

	nomcorrelation "github.com/theapemachine/nomagique/correlation"
)

/*
hayashiMoments computes covariance and both quadratic variations on exactly the
intervals that participate in at least one asynchronous overlap. Unmatched
prefixes and suffixes therefore cannot dilute an otherwise well-supported pair.
*/
func hayashiMoments(
	left, right []nomcorrelation.Sample,
	leftLogs, rightLogs []float64,
) (covariance, leftVariance, rightVariance float64, support int) {
	if len(left) < 2 || len(right) < 2 ||
		len(leftLogs) != len(left) || len(rightLogs) != len(right) {
		return 0, 0, 0, 0
	}

	leftSupported := make([]bool, len(left)-1)
	rightSupported := make([]bool, len(right)-1)
	overlaps := 0

	for leftIndex := 0; leftIndex < len(left)-1; leftIndex++ {
		if !hayashiIntervalOK(left[leftIndex], left[leftIndex+1]) {
			continue
		}

		leftReturn := leftLogs[leftIndex+1] - leftLogs[leftIndex]

		for rightIndex := 0; rightIndex < len(right)-1; rightIndex++ {
			if !hayashiIntervalOK(right[rightIndex], right[rightIndex+1]) {
				continue
			}

			if !left[leftIndex].At.Before(right[rightIndex+1].At) ||
				!right[rightIndex].At.Before(left[leftIndex+1].At) {
				continue
			}

			rightReturn := rightLogs[rightIndex+1] - rightLogs[rightIndex]
			covariance += leftReturn * rightReturn
			leftSupported[leftIndex] = true
			rightSupported[rightIndex] = true
			overlaps++
		}
	}

	leftCount := 0

	for index, supported := range leftSupported {
		if !supported {
			continue
		}

		leftReturn := leftLogs[index+1] - leftLogs[index]
		leftVariance += leftReturn * leftReturn
		leftCount++
	}

	rightCount := 0

	for index, supported := range rightSupported {
		if !supported {
			continue
		}

		rightReturn := rightLogs[index+1] - rightLogs[index]
		rightVariance += rightReturn * rightReturn
		rightCount++
	}

	if leftCount < 2 || rightCount < 2 || overlaps < 2 {
		return covariance, leftVariance, rightVariance, 0
	}

	return covariance, leftVariance, rightVariance, min(leftCount, rightCount)
}

func supportedCorrelation(
	left, right []nomcorrelation.Sample,
	leftLogs, rightLogs []float64,
) (float64, int, bool) {
	covariance, leftVariance, rightVariance, support := hayashiMoments(
		left, right, leftLogs, rightLogs,
	)

	if support == 0 || leftVariance <= 0 || rightVariance <= 0 {
		return 0, 0, false
	}

	denominator := math.Sqrt(leftVariance * rightVariance)

	if denominator <= 0 {
		return 0, 0, false
	}

	return min(1, max(-1, covariance/denominator)), support, true
}

/*
hayashiIntervalOK mirrors the unrestricted (maxInterval <= 0) path used by the
shared Hayashi estimator.
*/
func hayashiIntervalOK(previous, current nomcorrelation.Sample) bool {
	return previous.Value > 0 && current.Value > 0 && previous.At.Before(current.At)
}
