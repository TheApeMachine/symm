package stats

import "math"

/*
SortFloats sorts values in ascending order using insertion sort.
*/
func SortFloats(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}

/*
Median returns the median of a copy of values.
*/
func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	cp := append([]float64(nil), values...)
	SortFloats(cp)

	return PercentileSorted(cp, 0.5)
}

/*
PercentileSorted returns a quantile from an ascending-sorted slice.
*/
func PercentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

/*
Quartiles returns the lower and upper quartiles of values.
*/
func Quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	cp := append([]float64(nil), values...)
	SortFloats(cp)

	return PercentileSorted(cp, 0.25), PercentileSorted(cp, 0.75)
}

/*
Max returns the maximum value.
*/
func Max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak
}

/*
CrossSectionMedian returns the median of a batch of scores.
*/
func CrossSectionMedian(values []float64) float64 {
	return Median(values)
}

/*
MedianAbsoluteDeviation returns MAD from a pre-sorted slice and its median.
*/
func MedianAbsoluteDeviation(sorted []float64, median float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	deviations := make([]float64, len(sorted))

	for index, score := range sorted {
		deviations[index] = math.Abs(score - median)
	}

	SortFloats(deviations)

	return PercentileSorted(deviations, 0.5)
}

/*
CopySorted returns an ascending-sorted copy of values.
*/
func CopySorted(values []float64) []float64 {
	cp := append([]float64(nil), values...)
	SortFloats(cp)

	return cp
}
