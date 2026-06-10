package causal

import "math"

/*
hyInterval is a log-return realised over the half-open time interval (start, end], measured
in Unix nanoseconds. The Hayashi-Yoshida estimator sums products of returns whose intervals
overlap, which is exactly what makes it a consistent covariance estimator for assets sampled
asynchronously — no resampling onto a common clock, so no bias from non-synchronous trading.
*/
type hyInterval struct {
	start int64
	end   int64
	ret   float64
}

/*
hyReturns accumulates a bounded, time-ordered series of log-return intervals for one asset,
built directly from its asynchronous trade prints.
*/
type hyReturns struct {
	intervals []hyInterval
	capacity  int
	lastPrice float64
	lastNanos int64
}

func newHYReturns(capacity int) *hyReturns {
	if capacity < 1 {
		capacity = 1
	}

	return &hyReturns{
		intervals: make([]hyInterval, 0, capacity),
		capacity:  capacity,
	}
}

/*
Observe folds one trade print. The first print only seeds the price/time anchor; from the
second onward each consecutive pair yields one return interval. Out-of-order or duplicate
timestamps advance the price anchor without emitting a zero- or negative-width interval.
*/
func (series *hyReturns) Observe(nanos int64, price float64) {
	if price <= 0 {
		return
	}

	if series.lastPrice <= 0 || series.lastNanos <= 0 {
		series.lastPrice = price
		series.lastNanos = nanos

		return
	}

	if nanos <= series.lastNanos {
		series.lastPrice = price

		return
	}

	series.intervals = append(series.intervals, hyInterval{
		start: series.lastNanos,
		end:   nanos,
		ret:   math.Log(price / series.lastPrice),
	})

	if len(series.intervals) > series.capacity {
		series.intervals = series.intervals[len(series.intervals)-series.capacity:]
	}

	series.lastPrice = price
	series.lastNanos = nanos
}

func (series *hyReturns) len() int {
	return len(series.intervals)
}

/*
trim keeps only the most recent keep intervals, dropping stale history after a
volatility shock so contagion can recover once the market stabilizes.
*/
func (series *hyReturns) trim(keep int) {
	if series == nil {
		return
	}

	if keep <= 0 {
		series.intervals = series.intervals[:0]

		return
	}

	if len(series.intervals) <= keep {
		return
	}

	series.intervals = series.intervals[len(series.intervals)-keep:]
}

/*
lastReturnMagnitude is the absolute log return of the most recent interval.
*/
func (series *hyReturns) lastReturnMagnitude() float64 {
	if series == nil || len(series.intervals) == 0 {
		return 0
	}

	last := series.intervals[len(series.intervals)-1]

	if last.ret >= 0 {
		return last.ret
	}

	return -last.ret
}

/*
realizedVolatility is the root mean square of interval log returns.
*/
func (series *hyReturns) realizedVolatility() float64 {
	if series == nil || len(series.intervals) == 0 {
		return 0
	}

	total := series.realisedVariance()

	return math.Sqrt(total / float64(len(series.intervals)))
}

/*
realizedVolatilityExcludingLast estimates vol before the most recent print so a
macro shock is not measured against a baseline it just contaminated.
*/
func (series *hyReturns) realizedVolatilityExcludingLast() float64 {
	if series == nil || len(series.intervals) <= 1 {
		return series.realizedVolatility()
	}

	total := 0.0
	count := len(series.intervals) - 1

	for index := 0; index < count; index++ {
		ret := series.intervals[index].ret
		total += ret * ret
	}

	return math.Sqrt(total / float64(count))
}

/*
clone returns an independent snapshot so cross-asset correlation can be computed outside the
owning symbol's lock without racing further trade prints.
*/
func (series *hyReturns) clone() *hyReturns {
	if series == nil {
		return nil
	}

	copied := make([]hyInterval, len(series.intervals))
	copy(copied, series.intervals)

	return &hyReturns{
		intervals: copied,
		capacity:  series.capacity,
		lastPrice: series.lastPrice,
		lastNanos: series.lastNanos,
	}
}

func (series *hyReturns) cloneTail(window int) *hyReturns {
	cloned := series.clone()

	if cloned == nil {
		return nil
	}

	if window <= 0 {
		cloned.intervals = cloned.intervals[:0]

		return cloned
	}

	if len(cloned.intervals) > window {
		cloned.intervals = cloned.intervals[len(cloned.intervals)-window:]
	}

	return cloned
}

/*
realisedVariance is the Hayashi-Yoshida variance of the series against itself. Consecutive
intervals share only their endpoints (zero-width overlap), so only self-products survive and
the estimator collapses to the sum of squared returns.
*/
func (series *hyReturns) realisedVariance() float64 {
	total := 0.0

	for _, interval := range series.intervals {
		total += interval.ret * interval.ret
	}

	return total
}

/*
hayashiYoshidaCovariance sums the products of returns over every overlapping pair of intervals.
Both interval lists are time-ordered by start, so a single advancing window pointer keeps the
sweep close to linear in the combined length.
*/
func hayashiYoshidaCovariance(left, right []hyInterval) float64 {
	covariance := 0.0
	window := 0

	for _, leftInterval := range left {
		for window < len(right) && right[window].end <= leftInterval.start {
			window++
		}

		for index := window; index < len(right) && right[index].start < leftInterval.end; index++ {
			covariance += leftInterval.ret * right[index].ret
		}
	}

	return covariance
}

/*
hayashiYoshidaCorrelation normalises the asynchronous covariance by the two realised standard
deviations. It reports false when either series carries no variance, so callers never divide a
quiet book into a spurious correlation.
*/
func hayashiYoshidaCorrelation(left, right *hyReturns) (float64, bool) {
	if left == nil || right == nil {
		return 0, false
	}

	varLeft := left.realisedVariance()
	varRight := right.realisedVariance()

	if varLeft <= 0 || varRight <= 0 {
		return 0, false
	}

	correlation := hayashiYoshidaCovariance(left.intervals, right.intervals) / math.Sqrt(varLeft*varRight)

	if correlation > 1 {
		return 1, true
	}

	if correlation < -1 {
		return -1, true
	}

	return correlation, true
}
