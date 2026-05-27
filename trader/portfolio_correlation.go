package trader

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
)

type priceSample struct {
	at    time.Time
	price float64
}

type priceSampleRing struct {
	samples []priceSample
	head    int
	count   int
}

func newPriceSampleRing(capacity int) priceSampleRing {
	if capacity <= 0 {
		capacity = 1
	}

	return priceSampleRing{samples: make([]priceSample, capacity)}
}

func (sampleRing *priceSampleRing) push(at time.Time, price float64) {
	if at.IsZero() || price <= 0 {
		return
	}

	capacity := len(sampleRing.samples)
	sampleRing.samples[sampleRing.head] = priceSample{at: at, price: price}
	sampleRing.head = (sampleRing.head + 1) % capacity

	if sampleRing.count < capacity {
		sampleRing.count++
	}
}

func (sampleRing priceSampleRing) ordered() []priceSample {
	if sampleRing.count == 0 {
		return nil
	}

	ordered := make([]priceSample, sampleRing.count)
	start := sampleRing.startIndex()

	for index := 0; index < sampleRing.count; index++ {
		ordered[index] = sampleRing.samples[(start+index)%len(sampleRing.samples)]
	}

	return ordered
}

func (sampleRing priceSampleRing) startIndex() int {
	if sampleRing.count < len(sampleRing.samples) {
		return 0
	}

	return sampleRing.head
}

func correlationBarInterval() time.Duration {
	seconds := config.System.CorrelationBarSeconds

	if seconds <= 0 {
		seconds = config.System.CandleSeconds
	}

	if seconds <= 0 {
		return time.Second
	}

	return time.Duration(seconds) * time.Second
}

func synchronizedLogReturns(left, right []priceSample, interval time.Duration) ([]float64, []float64, bool) {
	if interval <= 0 || len(left) < 2 || len(right) < 2 {
		return nil, nil, false
	}

	overlapStart := left[0].at

	if right[0].at.After(overlapStart) {
		overlapStart = right[0].at
	}

	overlapEnd := left[len(left)-1].at

	if right[len(right)-1].at.Before(overlapEnd) {
		overlapEnd = right[len(right)-1].at
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

	return logReturnsFromPrices(leftPrices), logReturnsFromPrices(rightPrices), true
}

func forwardFillGrid(samples []priceSample, start, end time.Time, interval time.Duration) []float64 {
	prices := make([]float64, 0)
	sampleIndex := 0
	currentPrice := 0.0

	for grid := start; !grid.After(end); grid = grid.Add(interval) {
		for sampleIndex < len(samples) && !samples[sampleIndex].at.After(grid) {
			if samples[sampleIndex].price > 0 {
				currentPrice = samples[sampleIndex].price
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

func logReturnsFromPrices(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}

	returns := make([]float64, len(prices)-1)

	for index := 1; index < len(prices); index++ {
		returns[index-1] = math.Log(prices[index] / prices[index-1])
	}

	return returns
}
