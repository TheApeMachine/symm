package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCorrelation   = types.MustIntern("correlation")
	SymbolCovariance    = types.MustIntern("covariance")
	SymbolLeftVariance  = types.MustIntern("correlation/left_variance")
	SymbolRightVariance = types.MustIntern("correlation/right_variance")
	SymbolSupport       = types.MustIntern("support")
	SymbolReady         = types.MustIntern("ready")
	SymbolLeftShift     = types.MustIntern("correlation/left_shift_nanos")
	SymbolLeftReturns   = types.MustIntern("correlation/left_return_count")
	SymbolRightReturns  = types.MustIntern("correlation/right_return_count")
	SymbolLeftFromNanos = types.MustIntern("correlation/left_from_nanos")
	SymbolLeftToNanos   = types.MustIntern("correlation/left_to_nanos")
	SymbolRightFromNanos = types.MustIntern("correlation/right_from_nanos")
	SymbolRightToNanos  = types.MustIntern("correlation/right_to_nanos")
	SymbolLeftEnergy    = types.MustIntern("correlation/left_energy_rate")
	SymbolRightEnergy   = types.MustIntern("correlation/right_energy_rate")
)

/*
Hayashi returns the primitive that evaluates two asynchronous path series —
leftPrefix and rightPrefix in one Frame — over every overlapping return
interval, without resampling either path onto an invented clock. Neither
projection is mutated.
*/
func Hayashi(leftPrefix string, rightPrefix string) types.Primitive {
	leftSeries := temporal.NewSeries(leftPrefix)
	rightSeries := temporal.NewSeries(rightPrefix)

	return func(input *types.Frame) {
		shiftValue, hasShift := input.Get(SymbolLeftShift)

		if hasShift && shiftValue != math.Trunc(shiftValue) {
			input.Err = correlationError(
				"left shift must contain integral nanoseconds",
			)

			return
		}

		left, leftCount := seriesPoints(leftSeries, input)
		right, rightCount := seriesPoints(rightSeries, input)
		leftReturns, leftReturnCount, leftVariance := seriesReturns(&left, leftCount)
		rightReturns, rightReturnCount, rightVariance := seriesReturns(&right, rightCount)

		correlation, covariance, leftVariance, rightVariance, support, ready :=
			hayashiPoints(&leftReturns, leftReturnCount, leftVariance, &rightReturns, rightReturnCount, rightVariance, int64(shiftValue))

		input.Put(SymbolCorrelation, correlation)
		input.Put(SymbolCovariance, covariance)
		input.Put(SymbolLeftVariance, leftVariance)
		input.Put(SymbolRightVariance, rightVariance)
		input.Put(SymbolSupport, float64(support))
		input.Put(SymbolReady, truth(ready))

		if leftCount > 1 {
			input.Put(SymbolLeftReturns, float64(leftCount-1))
			input.Put(SymbolLeftFromNanos, float64(left[0].timestamp))
			input.Put(SymbolLeftToNanos, float64(left[leftCount-1].timestamp))
			input.Put(SymbolLeftEnergy, pathEnergyRate(&left, leftCount))
		}

		if rightCount > 1 {
			input.Put(SymbolRightReturns, float64(rightCount-1))
			input.Put(SymbolRightFromNanos, float64(right[0].timestamp))
			input.Put(SymbolRightToNanos, float64(right[rightCount-1].timestamp))
			input.Put(SymbolRightEnergy, pathEnergyRate(&right, rightCount))
		}
	}
}

type point struct {
	timestamp int64
	value     float64
}

func seriesPoints(
	series temporal.Series,
	path *types.Frame,
) ([temporal.MaxPathSamples]point, int) {
	points := [temporal.MaxPathSamples]point{}
	count := series.Count(*path)

	for index := 0; index < count; index++ {
		timestamp, value, found := series.Sample(path, index)

		if !found {
			continue
		}

		points[index] = point{timestamp: timestamp, value: value}
	}

	return points, count
}

type returnInterval struct {
	from int64
	to   int64
	val  float64
}

func seriesReturns(points *[temporal.MaxPathSamples]point, count int) ([temporal.MaxPathSamples]returnInterval, int, float64) {
	returns := [temporal.MaxPathSamples]returnInterval{}
	returnCount := 0
	variance := 0.0

	for index := 1; index < count; index++ {
		previous := points[index-1]
		current := points[index]

		if previous.timestamp >= current.timestamp ||
			previous.value <= 0 || current.value <= 0 {
			continue
		}

		val := math.Log(current.value / previous.value)
		returns[returnCount] = returnInterval{
			from: previous.timestamp,
			to:   current.timestamp,
			val:  val,
		}
		returnCount++
		variance += val * val
	}

	return returns, returnCount, variance
}



/*
pathEnergyRate returns the median interval-normalized log-return energy of one
path: the median of r²/Δt over its valid return intervals. It is the robust
typical return-energy rate the path contributes to a pair.
*/
func pathEnergyRate(points *[temporal.MaxPathSamples]point, count int) float64 {
	rates := [temporal.MaxPathSamples]float64{}
	rateCount := 0

	for index := 1; index < count; index++ {
		previous := points[index-1]
		current := points[index]

		if previous.timestamp >= current.timestamp ||
			previous.value <= 0 || current.value <= 0 {
			continue
		}

		returnValue := math.Log(current.value / previous.value)
		elapsed := float64(current.timestamp-previous.timestamp) / 1e9

		if elapsed <= 0 {
			continue
		}

		rates[rateCount] = returnValue * returnValue / elapsed
		rateCount++
	}

	if rateCount == 0 {
		return 0
	}

	for index := 1; index < rateCount; index++ {
		value := rates[index]
		position := index

		for position > 0 && rates[position-1] > value {
			rates[position] = rates[position-1]
			position--
		}

		rates[position] = value
	}

	middle := rateCount / 2

	if rateCount%2 == 0 {
		return (rates[middle-1] + rates[middle]) / 2
	}

	return rates[middle]
}

func hayashiPoints(
	left *[temporal.MaxPathSamples]returnInterval,
	leftCount int,
	leftVariance float64,
	right *[temporal.MaxPathSamples]returnInterval,
	rightCount int,
	rightVariance float64,
	leftShift int64,
) (float64, float64, float64, float64, int, bool) {
	covariance := 0.0
	support := 0
	rightStart := 0

	for leftIndex := 0; leftIndex < leftCount; leftIndex++ {
		leftInterval := left[leftIndex]
		leftFrom := leftInterval.from + leftShift
		leftTo := leftInterval.to + leftShift

		for rightStart < rightCount {
			if leftFrom >= right[rightStart].to {
				rightStart++
				continue
			}
			break
		}

		for rightIndex := rightStart; rightIndex < rightCount; rightIndex++ {
			rightInterval := right[rightIndex]

			if rightInterval.from >= leftTo {
				break
			}

			covariance += leftInterval.val * rightInterval.val
			support++
		}
	}

	if support == 0 || leftVariance <= 0 || rightVariance <= 0 {
		return 0, covariance, leftVariance, rightVariance, support, false
	}

	correlation := covariance / math.Sqrt(leftVariance*rightVariance)
	correlation = math.Max(-1, math.Min(1, correlation))

	return correlation, covariance, leftVariance, rightVariance, support, true
}

func truth(value bool) float64 {
	if value {
		return 1
	}

	return 0
}

func correlationError(message string) error {
	return &hayashiError{message: message}
}

type hayashiError struct {
	message string
}

func (err *hayashiError) Error() string {
	return "correlation: hayashi " + err.message
}
