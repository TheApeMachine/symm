package statutil

import (
	"math"
	"sort"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/stat"
)

func Quantile(percentile float64, values []float64) (float64, error) {
	if len(values) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"statutil: quantile values required",
			nil,
		))
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	for _, value := range sorted {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"statutil: quantile sample is non-finite",
				nil,
			))
		}
	}

	return stat.Quantile(percentile, stat.LinInterp, sorted, nil), nil
}

func Quartiles(values []float64) (lower float64, upper float64, err error) {
	if len(values) == 0 {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"statutil: quartiles values required",
			nil,
		))
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	for _, value := range sorted {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"statutil: quartiles sample is non-finite",
				nil,
			))
		}
	}

	lower = stat.Quantile(0.25, stat.LinInterp, sorted, nil)
	upper = stat.Quantile(0.75, stat.LinInterp, sorted, nil)

	return lower, upper, nil
}

/*
MedianAbsoluteDeviation returns the median absolute deviation from center.
When center is zero the sample median is used.
*/
func MedianAbsoluteDeviation(values []float64, center float64) float64 {
	if len(values) == 0 {
		return 0
	}

	if center == 0 {
		center = Median(values)
	}

	deviations := make([]float64, len(values))

	for index, value := range values {
		deviation := value - center

		if deviation < 0 {
			deviation = -deviation
		}

		deviations[index] = deviation
	}

	return Median(deviations)
}

func DistinctSpan(values []float64) (float64, error) {
	if len(values) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"statutil: span values required",
			nil,
		))
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	distinct := 1

	for index := 1; index < len(sorted); index++ {
		if sorted[index] == sorted[index-1] {
			continue
		}

		distinct++
	}

	if distinct <= 1 {
		return 0, nil
	}

	return float64(distinct), nil
}
