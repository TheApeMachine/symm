package correlation

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
)

const maxHayashiYoshidaInterval = 5 * time.Minute

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
HayashiYoshidaCorrelation estimates asynchronous high-frequency correlation
with a sliding sweep over overlapping return intervals. It does not require
both symbols to trade inside the same grid bar.
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
ShiftPriceSamples moves timestamps by offset without changing prices. Lead-lag
scoring uses this to test whether an anchor path explains a later follower path.
*/
func ShiftPriceSamples(samples []PriceSample, offset time.Duration) []PriceSample {
	if len(samples) == 0 || offset == 0 {
		return append([]PriceSample(nil), samples...)
	}

	shifted := make([]PriceSample, len(samples))

	for index := range samples {
		shifted[index] = PriceSample{
			At:    samples[index].At.Add(offset),
			Price: samples[index].Price,
		}
	}

	return shifted
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
