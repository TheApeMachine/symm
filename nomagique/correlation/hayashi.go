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

	return func(input types.Frame) types.Frame {
		shiftValue, hasShift := input.Get(SymbolLeftShift)

		if hasShift && shiftValue != math.Trunc(shiftValue) {
			input.Err = correlationError(
				"left shift must contain integral nanoseconds",
			)

			return input
		}

		left, leftCount := seriesPoints(leftSeries, &input)
		right, rightCount := seriesPoints(rightSeries, &input)
		correlation, covariance, leftVariance, rightVariance, support, ready :=
			hayashiPoints(&left, leftCount, &right, rightCount, int64(shiftValue))

		input.Put(SymbolCorrelation, correlation)
		input.Put(SymbolCovariance, covariance)
		input.Put(SymbolLeftVariance, leftVariance)
		input.Put(SymbolRightVariance, rightVariance)
		input.Put(SymbolSupport, float64(support))
		input.Put(SymbolReady, truth(ready))

		return input
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

func pathVariance(points *[temporal.MaxPathSamples]point, count int) float64 {
	variance := 0.0

	for index := 1; index < count; index++ {
		previous := points[index-1]
		current := points[index]

		if previous.timestamp >= current.timestamp ||
			previous.value <= 0 || current.value <= 0 {
			continue
		}

		value := math.Log(current.value / previous.value)
		variance += value * value
	}

	return variance
}

func hayashiPoints(
	left *[temporal.MaxPathSamples]point,
	leftCount int,
	right *[temporal.MaxPathSamples]point,
	rightCount int,
	leftShift int64,
) (float64, float64, float64, float64, int, bool) {
	leftVariance := pathVariance(left, leftCount)
	rightVariance := pathVariance(right, rightCount)
	covariance := 0.0
	support := 0
	rightStart := 0

	for leftIndex := 0; leftIndex < leftCount-1; leftIndex++ {
		leftFrom := left[leftIndex]
		leftTo := left[leftIndex+1]
		leftFrom.timestamp += leftShift
		leftTo.timestamp += leftShift

		if leftFrom.timestamp >= leftTo.timestamp ||
			leftFrom.value <= 0 || leftTo.value <= 0 {
			continue
		}

		for rightStart < rightCount-1 {
			if leftFrom.timestamp >= right[rightStart+1].timestamp {
				rightStart++
				continue
			}

			break
		}

		leftReturn := math.Log(leftTo.value / leftFrom.value)

		for rightIndex := rightStart; rightIndex < rightCount-1; rightIndex++ {
			rightFrom := right[rightIndex]
			rightTo := right[rightIndex+1]

			if rightFrom.timestamp >= leftTo.timestamp {
				break
			}

			if rightFrom.timestamp >= rightTo.timestamp ||
				rightFrom.value <= 0 || rightTo.value <= 0 {
				continue
			}

			covariance += leftReturn * math.Log(rightTo.value/rightFrom.value)
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
