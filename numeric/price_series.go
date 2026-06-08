package numeric

import (
	"math"
	"time"
)

// maxHayashiYoshidaInterval bounds how far apart two consecutive samples may sit
// before the interval between them is treated as non-contemporaneous and dropped
// from the estimator. It is a validity bound of the estimator, not a tunable.
const maxHayashiYoshidaInterval = 5 * time.Minute

/*
PriceSample is one timestamped price observation for return resampling.
*/
type PriceSample struct {
	At    time.Time
	Price float64
}

/*
PriceSampleRing is a fixed-capacity rolling window of price samples. It is the
substrate the asynchronous-correlation estimators read from.
*/
type PriceSampleRing struct {
	samples []PriceSample
	head    int
	count   int
}

/*
NewPriceSampleRing allocates one rolling window with the given capacity.
*/
func NewPriceSampleRing(capacity int) PriceSampleRing {
	if capacity <= 0 {
		capacity = 1
	}

	return PriceSampleRing{samples: make([]PriceSample, capacity)}
}

/*
Push records one price sample when the timestamp and price are valid.
*/
func (sampleRing *PriceSampleRing) Push(at time.Time, price float64) {
	if at.IsZero() || price <= 0 {
		return
	}

	capacity := len(sampleRing.samples)
	sampleRing.samples[sampleRing.head] = PriceSample{At: at, Price: price}
	sampleRing.head = (sampleRing.head + 1) % capacity

	if sampleRing.count < capacity {
		sampleRing.count++
	}
}

/*
AppendOrdered appends the window contents from oldest to newest into destination.
Callers on hot paths can pass reusable storage to avoid per-read heap churn.
*/
func (sampleRing PriceSampleRing) AppendOrdered(destination []PriceSample) []PriceSample {
	if sampleRing.count == 0 {
		return destination[:0]
	}

	if cap(destination) < sampleRing.count {
		destination = make([]PriceSample, 0, sampleRing.count)
	}

	ordered := destination[:0]
	start := sampleRing.startIndex()

	for index := 0; index < sampleRing.count; index++ {
		ordered = append(
			ordered, sampleRing.samples[(start+index)%len(sampleRing.samples)],
		)
	}

	return ordered
}

func (sampleRing PriceSampleRing) startIndex() int {
	if sampleRing.count < len(sampleRing.samples) {
		return 0
	}

	return sampleRing.head
}

/*
HayashiYoshidaCorrelation estimates asynchronous high-frequency correlation with
a sliding sweep over overlapping return intervals. It does not require both
series to trade inside the same grid bar.
*/
func HayashiYoshidaCorrelation(left, right []PriceSample) (float64, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, false
	}

	leftVariance := varianceSum(left)
	rightVariance := varianceSum(right)

	if leftVariance <= 0 || rightVariance <= 0 {
		return 0, false
	}

	covariance := 0.0
	rightStart := 0

	for leftIndex := 0; leftIndex < len(left)-1; leftIndex++ {
		leftStart := left[leftIndex].At
		leftEnd := left[leftIndex+1].At

		if !validHYInterval(left[leftIndex], left[leftIndex+1]) {
			continue
		}

		leftReturn := math.Log(left[leftIndex+1].Price / left[leftIndex].Price)

		for rightStart < len(right)-1 {
			if !validHYInterval(right[rightStart], right[rightStart+1]) ||
				!leftStart.Before(right[rightStart+1].At) {
				rightStart++
				continue
			}

			break
		}

		for rightIndex := rightStart; rightIndex < len(right)-1; rightIndex++ {
			rightIntervalStart := right[rightIndex].At

			if !rightIntervalStart.Before(leftEnd) {
				break
			}

			if !validHYInterval(right[rightIndex], right[rightIndex+1]) {
				continue
			}

			covariance += leftReturn * math.Log(
				right[rightIndex+1].Price/right[rightIndex].Price,
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

func varianceSum(samples []PriceSample) float64 {
	if len(samples) < 2 {
		return 0
	}

	sum := 0.0

	for index := 1; index < len(samples); index++ {
		if !validHYInterval(samples[index-1], samples[index]) {
			continue
		}

		ret := math.Log(samples[index].Price / samples[index-1].Price)
		sum += ret * ret
	}

	return sum
}

func validHYInterval(previous, current PriceSample) bool {
	if previous.Price <= 0 || current.Price <= 0 || !previous.At.Before(current.At) {
		return false
	}

	return current.At.Sub(previous.At) <= maxHayashiYoshidaInterval
}

/*
Pearson computes the sample correlation between two equal-length series.
*/
func Pearson(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}

	leftMean := 0.0
	rightMean := 0.0

	for index := range left {
		leftMean += left[index]
		rightMean += right[index]
	}

	sampleCount := float64(len(left))
	leftMean /= sampleCount
	rightMean /= sampleCount

	covariance := 0.0
	leftVariance := 0.0
	rightVariance := 0.0

	for index := range left {
		leftDelta := left[index] - leftMean
		rightDelta := right[index] - rightMean

		covariance += leftDelta * rightDelta
		leftVariance += leftDelta * leftDelta
		rightVariance += rightDelta * rightDelta
	}

	denominator := math.Sqrt(leftVariance * rightVariance)

	if denominator <= 0 {
		return 0
	}

	return covariance / denominator
}
