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

	leftPrices, leftPresent := observedGrid(left, gridStart, overlapEnd, interval)
	rightPrices, rightPresent := observedGrid(right, gridStart, overlapEnd, interval)

	if len(leftPrices) != len(rightPrices) || len(leftPrices) < 2 {
		return nil, nil, false
	}

	leftReturns, rightReturns := alignedLogReturns(leftPrices, rightPrices, leftPresent, rightPresent)

	if len(leftReturns) < 2 {
		return nil, nil, false
	}

	return leftReturns, rightReturns, true
}

/*
observedGrid carries the last seen price across the grid for indexing but
also returns a parallel slice of "this bar saw a fresh sample" flags. The
flag is what alignedLogReturns uses to skip bars on either side that had no
fresh observation — forward-filling the price into the bar would otherwise
fabricate a zero return that depresses the illiquid leg's variance and
biases the Pearson denominator toward 0.
*/
func observedGrid(samples []PriceSample, start, end time.Time, interval time.Duration) ([]float64, []bool) {
	prices := make([]float64, 0)
	present := make([]bool, 0)
	sampleIndex := 0
	currentPrice := 0.0

	for grid := start; !grid.After(end); grid = grid.Add(interval) {
		freshThisBar := false

		for sampleIndex < len(samples) && !samples[sampleIndex].At.After(grid) {
			if samples[sampleIndex].Price > 0 {
				currentPrice = samples[sampleIndex].Price
				freshThisBar = true
			}

			sampleIndex++
		}

		if currentPrice <= 0 {
			continue
		}

		prices = append(prices, currentPrice)
		present = append(present, freshThisBar)
	}

	return prices, present
}

/*
alignedLogReturns emits a log-return only when both sides saw a fresh
observation in the bar pair that spans the return. A bar with no fresh
sample is treated as missing — its return is dropped rather than zeroed —
so the resulting Pearson is computed over genuinely observed comovement.
*/
func alignedLogReturns(left, right []float64, leftPresent, rightPresent []bool) ([]float64, []float64) {
	if len(left) != len(right) || len(left) < 2 {
		return nil, nil
	}

	leftReturns := make([]float64, 0, len(left)-1)
	rightReturns := make([]float64, 0, len(right)-1)

	for index := 1; index < len(left); index++ {
		// A return at index spans bar[index-1] → bar[index]; both endpoints
		// must be fresh observations. If only the current bar is fresh and
		// the prior was forward-filled, the computed log return would
		// straddle a stale bar and re-introduce the fabricated-zero bias
		// the present check is meant to avoid.
		if !leftPresent[index] || !rightPresent[index] {
			continue
		}

		if !leftPresent[index-1] || !rightPresent[index-1] {
			continue
		}

		if left[index-1] <= 0 || right[index-1] <= 0 {
			continue
		}

		leftReturns = append(leftReturns, math.Log(left[index]/left[index-1]))
		rightReturns = append(rightReturns, math.Log(right[index]/right[index-1]))
	}

	return leftReturns, rightReturns
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
