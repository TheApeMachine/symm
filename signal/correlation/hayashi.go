package correlation

import (
	"math"
	"time"

	nomcorrelation "github.com/theapemachine/nomagique/correlation"
)

/*
hayashiCovariance returns the Hayashi-Yoshida cross product sum matching
nomagique's pairwise estimator, without normalizing by the variance terms.
Log-prices are precomputed so each return is a subtraction.
*/
func hayashiCovariance(
	left, right []nomcorrelation.Sample,
	leftLogs, rightLogs []float64,
) float64 {
	if len(left) < 2 || len(right) < 2 ||
		len(leftLogs) != len(left) || len(rightLogs) != len(right) {
		return 0
	}

	covariance := 0.0
	rightStart := 0

	for leftIndex := 0; leftIndex < len(left)-1; leftIndex++ {
		leftStart := left[leftIndex].At
		leftEnd := left[leftIndex+1].At

		if !hayashiIntervalOK(left[leftIndex], left[leftIndex+1]) {
			continue
		}

		leftReturn := leftLogs[leftIndex+1] - leftLogs[leftIndex]

		for rightStart < len(right)-1 {
			if !hayashiIntervalOK(right[rightStart], right[rightStart+1]) ||
				!leftStart.Before(right[rightStart+1].At) {
				rightStart++

				continue
			}

			break
		}

		for rightIndex := rightStart; rightIndex < len(right)-1; rightIndex++ {
			if !right[rightIndex].At.Before(leftEnd) {
				break
			}

			if !hayashiIntervalOK(right[rightIndex], right[rightIndex+1]) {
				continue
			}

			covariance += leftReturn * (rightLogs[rightIndex+1] - rightLogs[rightIndex])
		}
	}

	return covariance
}

/*
overlapCovarianceRecent returns the cross-product of one newest left interval
against overlapping right intervals by scanning backward from the right tail.
Ancient right intervals end before leftStart, so the walk breaks after the
live overlap region instead of skipping them from the head.
*/
func overlapCovarianceRecent(
	leftReturn float64,
	leftStart, leftEnd time.Time,
	right []nomcorrelation.Sample,
	rightLogs []float64,
) float64 {
	if len(right) < 2 || leftReturn == 0 || !leftStart.Before(leftEnd) ||
		len(rightLogs) != len(right) {
		return 0
	}

	covariance := 0.0

	for index := len(right) - 2; index >= 0; index-- {
		intervalStart := right[index].At
		intervalEnd := right[index+1].At

		if !leftStart.Before(intervalEnd) {
			break
		}

		if !intervalStart.Before(leftEnd) {
			continue
		}

		if !hayashiIntervalOK(right[index], right[index+1]) {
			continue
		}

		covariance += leftReturn * (rightLogs[index+1] - rightLogs[index])
	}

	return covariance
}

/*
overlapCovarianceRetired returns the cross-product of one left-edge interval
being evicted. The retired interval lives at the head of the timeline, so the
scan walks forward until right intervals start at or after leftEnd.
*/
func overlapCovarianceRetired(
	leftReturn float64,
	leftStart, leftEnd time.Time,
	right []nomcorrelation.Sample,
	rightLogs []float64,
) float64 {
	if len(right) < 2 || leftReturn == 0 || !leftStart.Before(leftEnd) ||
		len(rightLogs) != len(right) {
		return 0
	}

	covariance := 0.0

	for index := 0; index < len(right)-1; index++ {
		intervalStart := right[index].At
		intervalEnd := right[index+1].At

		if !intervalStart.Before(leftEnd) {
			break
		}

		if !leftStart.Before(intervalEnd) {
			continue
		}

		if !hayashiIntervalOK(right[index], right[index+1]) {
			continue
		}

		covariance += leftReturn * (rightLogs[index+1] - rightLogs[index])
	}

	return covariance
}

/*
hayashiIntervalOK mirrors the unrestricted (maxInterval <= 0) path used by the
shared Hayashi estimator.
*/
func hayashiIntervalOK(previous, current nomcorrelation.Sample) bool {
	return previous.Value > 0 && current.Value > 0 && previous.At.Before(current.At)
}

/*
pairCorrelation normalizes stored covariance by the live symbol variances.
*/
func pairCorrelation(
	pair *pairState,
	leftVariance, rightVariance float64,
) (float64, bool) {
	if pair == nil || leftVariance <= 0 || rightVariance <= 0 {
		return 0, false
	}

	denominator := math.Sqrt(leftVariance * rightVariance)

	if denominator <= 0 {
		return 0, false
	}

	correlationValue := pair.covariance / denominator

	if correlationValue > 1 {
		return 1, true
	}

	if correlationValue < -1 {
		return -1, true
	}

	return correlationValue, true
}
