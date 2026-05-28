package correlation

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
)

/*
PriceSample is one timestamped price observation for return resampling.
*/
type PriceSample struct {
	At    time.Time
	Price float64
}

/*
PriceSampleRing stores a fixed-capacity rolling price window.
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
Ordered returns the window contents from oldest to newest.
*/
func (sampleRing PriceSampleRing) Ordered() []PriceSample {
	if sampleRing.count == 0 {
		return nil
	}

	ordered := make([]PriceSample, sampleRing.count)
	start := sampleRing.startIndex()

	for index := 0; index < sampleRing.count; index++ {
		ordered[index] = sampleRing.samples[(start+index)%len(sampleRing.samples)]
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
BarInterval returns the grid step used for synchronized return sampling.
*/
func BarInterval() time.Duration {
	seconds := config.System.CorrelationBarSeconds

	if seconds <= 0 {
		seconds = config.System.CandleSeconds
	}

	if seconds <= 0 {
		return time.Second
	}

	return time.Duration(seconds) * time.Second
}

/*
SynchronizedLogReturns aligns two price windows on a shared time grid.
*/
func SynchronizedLogReturns(
	left, right []PriceSample,
	interval time.Duration,
) ([]float64, []float64, bool) {
	if interval <= 0 || len(left) < 2 || len(right) < 2 {
		return nil, nil, false
	}

	overlapStart := left[0].At

	if right[0].At.After(overlapStart) {
		overlapStart = right[0].At
	}

	overlapEnd := left[len(left)-1].At

	if right[len(right)-1].At.Before(overlapEnd) {
		overlapEnd = right[len(right)-1].At
	}

	if !overlapStart.Before(overlapEnd) {
		return nil, nil, false
	}

	gridStart := overlapStart.Truncate(interval)

	if gridStart.Before(overlapStart) {
		gridStart = gridStart.Add(interval)
	}

	leftPrices := forwardFillGrid(left, gridStart, overlapEnd, interval)
	rightPrices := forwardFillGrid(right, gridStart, overlapEnd, interval)

	if len(leftPrices) != len(rightPrices) || len(leftPrices) < 2 {
		return nil, nil, false
	}

	return LogReturnsFromPrices(leftPrices), LogReturnsFromPrices(rightPrices), true
}

func forwardFillGrid(samples []PriceSample, start, end time.Time, interval time.Duration) []float64 {
	prices := make([]float64, 0)
	sampleIndex := 0
	currentPrice := 0.0

	for grid := start; !grid.After(end); grid = grid.Add(interval) {
		for sampleIndex < len(samples) && !samples[sampleIndex].At.After(grid) {
			if samples[sampleIndex].Price > 0 {
				currentPrice = samples[sampleIndex].Price
			}

			sampleIndex++
		}

		if currentPrice <= 0 {
			continue
		}

		prices = append(prices, currentPrice)
	}

	return prices
}

/*
LogReturnsFromPrices converts a price path into log returns.
*/
func LogReturnsFromPrices(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}

	returns := make([]float64, len(prices)-1)

	for index := 1; index < len(prices); index++ {
		returns[index-1] = math.Log(prices[index] / prices[index-1])
	}

	return returns
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

	if leftVariance <= 0 || rightVariance <= 0 {
		return 0
	}

	return covariance / math.Sqrt(leftVariance*rightVariance)
}
