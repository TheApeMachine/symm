package correlation

import (
	"math"
	"time"
)

/*
Sample is one time-stamped observation in an asynchronous price path.
*/
type Sample struct {
	At    time.Time
	Value float64
}

/*
HayashiYoshida calculates correlation from every pair of overlapping return
intervals without resampling either input path onto an invented common clock.
*/
func HayashiYoshida(
	left []Sample,
	right []Sample,
	maxInterval time.Duration,
) (float64, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, false
	}

	leftVariance := varianceSum(left, maxInterval)
	rightVariance := varianceSum(right, maxInterval)

	if leftVariance <= 0 || rightVariance <= 0 {
		return 0, false
	}

	covariance := 0.0
	rightStart := 0

	for leftIndex := 0; leftIndex < len(left)-1; leftIndex++ {
		leftStart := left[leftIndex].At
		leftEnd := left[leftIndex+1].At

		if !validInterval(left[leftIndex], left[leftIndex+1], maxInterval) {
			continue
		}

		leftReturn := math.Log(left[leftIndex+1].Value / left[leftIndex].Value)

		for rightStart < len(right)-1 {
			if !validInterval(right[rightStart], right[rightStart+1], maxInterval) ||
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

			if !validInterval(right[rightIndex], right[rightIndex+1], maxInterval) {
				continue
			}

			covariance += leftReturn * math.Log(
				right[rightIndex+1].Value/right[rightIndex].Value,
			)
		}
	}

	denominator := math.Sqrt(leftVariance * rightVariance)

	if denominator <= 0 {
		return 0, false
	}

	correlation := covariance / denominator

	if correlation > 1 {
		return 1, true
	}

	if correlation < -1 {
		return -1, true
	}

	return correlation, true
}

func varianceSum(samples []Sample, maxInterval time.Duration) float64 {
	sum := 0.0

	for index := 1; index < len(samples); index++ {
		if !validInterval(samples[index-1], samples[index], maxInterval) {
			continue
		}

		value := math.Log(samples[index].Value / samples[index-1].Value)
		sum += value * value
	}

	return sum
}

func validInterval(left Sample, right Sample, maxInterval time.Duration) bool {
	if !left.At.Before(right.At) || left.Value <= 0 || right.Value <= 0 {
		return false
	}

	return maxInterval <= 0 || right.At.Sub(left.At) <= maxInterval
}
