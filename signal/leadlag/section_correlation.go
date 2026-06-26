package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/symm/statutil"
)

// pearsonFloor is the structural minimum for a correlation: two paired
// observations define a (degenerate) line. It is NOT a warmup gate — the first
// observation still emits (a single price seeds the path and correlation is
// reported low-confidence the moment a second spaced sample arrives). Below two
// points Pearson is undefined, so this floor is the math itself, not a tuned
// sample count.
const pearsonFloor = 2

func minCorrelationSamples(sampleCount int) int {
	if sampleCount < pearsonFloor {
		return pearsonFloor
	}

	return int(math.Ceil(math.Sqrt(float64(sampleCount))))
}

func priceHistoryCapacity(sampleCount int) int {
	if sampleCount <= 1 {
		return sampleCount
	}

	// Retention grows from the observed sample count: enough rows to fit the
	// adaptive correlation window plus its lag span. No fixed regime constant —
	// a thin path keeps everything, a deep path keeps its derived window.
	minSamples := minCorrelationSamples(sampleCount)
	lagBars := maxLagBarsForCount(sampleCount)
	retention := minSamples + lagBars + 1

	if retention > sampleCount {
		return sampleCount
	}

	return retention
}

func maxLagBarsForCount(sampleCount int) int {
	minSamples := minCorrelationSamples(sampleCount)

	if sampleCount < minSamples {
		return 1
	}

	bars := sampleCount / minSamples

	if bars < 1 {
		return 1
	}

	return bars
}

func recentPathMove(samples []priceSample, window time.Duration) (float64, bool) {
	minSamples := minCorrelationSamples(len(samples))

	if len(samples) < minSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.at.Add(-window)
	startIndex := -1

	for index, sample := range samples {
		if !sample.at.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.value <= 0 || latest.value <= 0 {
		return 0, false
	}

	if start.value == latest.value {
		return 0, true
	}

	spacing := seriesSampleSpacing(samples, nil)

	if spacing <= 0 || minSamples < pearsonFloor {
		return 0, false
	}

	minimumSpan := spacing * time.Duration(minSamples-1)

	if latest.at.Sub(start.at) < minimumSpan {
		return 0, false
	}

	return math.Abs(math.Log(latest.value / start.value)), true
}

func pairCorrelation(left, right []priceSample, lag time.Duration) (float64, bool) {
	leftReturns, rightReturns := alignedReturns(left, right, lag)

	if len(leftReturns) < minCorrelationSamples(len(leftReturns)) {
		return 0, false
	}

	correlation := pearson(leftReturns, rightReturns)

	if math.IsNaN(correlation) {
		return 0, false
	}

	return correlation, true
}

func crossLagScore(
	anchor, follower []priceSample,
	interval time.Duration,
	lagLimit int,
) (int, float64, bool) {
	bestBars := 0
	bestCorr := 0.0
	found := false

	if lagLimit < 1 {
		lagLimit = 1
	}

	for bars := 1; bars <= lagLimit; bars++ {
		lag := time.Duration(bars) * interval
		correlation, ok := pairCorrelation(anchor, follower, lag)

		if !ok {
			continue
		}

		if !found || math.Abs(correlation) > math.Abs(bestCorr) {
			bestBars = bars
			bestCorr = correlation
			found = true
		}
	}

	return bestBars, bestCorr, found
}

func alignedReturns(left, right []priceSample, lag time.Duration) ([]float64, []float64) {
	leftReturns := logReturns(left)
	rightReturns := logReturns(right)

	if lag == 0 {
		count := len(leftReturns)

		if len(rightReturns) < count {
			count = len(rightReturns)
		}

		if count < minCorrelationSamples(count) {
			return nil, nil
		}

		return leftReturns[len(leftReturns)-count:], rightReturns[len(rightReturns)-count:]
	}

	interval := medianSampleSpacing(left)

	if interval <= 0 {
		return nil, nil
	}

	shift := int(lag / interval)

	if shift <= 0 || shift >= len(leftReturns) || shift >= len(rightReturns) {
		return nil, nil
	}

	leftTail := leftReturns[:len(leftReturns)-shift]
	rightTail := rightReturns[shift:]
	count := len(leftTail)

	if len(rightTail) < count {
		count = len(rightTail)
	}

	if count < minCorrelationSamples(count) {
		return nil, nil
	}

	return leftTail[len(leftTail)-count:], rightTail[len(rightTail)-count:]
}

func logReturns(samples []priceSample) []float64 {
	if len(samples) < 2 {
		return nil
	}

	returns := make([]float64, 0, len(samples)-1)

	for index := 1; index < len(samples); index++ {
		previous := samples[index-1].value
		current := samples[index].value

		if previous <= 0 || current <= 0 {
			continue
		}

		returns = append(returns, math.Log(current/previous))
	}

	return returns
}

func pearson(left, right []float64) float64 {
	if len(left) != len(right) || len(left) == 0 {
		return math.NaN()
	}

	meanLeft, stdLeft := meanStdDev(left)
	meanRight, stdRight := meanStdDev(right)

	if stdLeft <= 0 || stdRight <= 0 {
		return math.NaN()
	}

	covariance := 0.0

	for index := range left {
		covariance += (left[index] - meanLeft) * (right[index] - meanRight)
	}

	covariance /= float64(len(left))

	return covariance / (stdLeft * stdRight)
}

func meanStdDev(values []float64) (mean float64, std float64) {
	if len(values) == 0 {
		return 0, 0
	}

	for _, value := range values {
		mean += value
	}

	mean /= float64(len(values))

	for _, value := range values {
		delta := value - mean
		std += delta * delta
	}

	std = math.Sqrt(std / float64(len(values)))

	return mean, std
}

func seriesSampleSpacing(primary, secondary []priceSample) time.Duration {
	spacing := medianSampleSpacing(primary)

	if len(secondary) > 1 {
		alternate := medianSampleSpacing(secondary)

		if alternate > 0 && (spacing <= 0 || alternate < spacing) {
			spacing = alternate
		}
	}

	if spacing <= 0 {
		return 0
	}

	return spacing
}

func medianSampleSpacing(samples []priceSample) time.Duration {
	if len(samples) < 2 {
		return 0
	}

	gaps := make([]float64, 0, len(samples)-1)

	for index := 1; index < len(samples); index++ {
		gap := samples[index].at.Sub(samples[index-1].at).Seconds()

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	return time.Duration(statutil.Median(gaps) * float64(time.Second))
}
