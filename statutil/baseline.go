package statutil

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

// SampleBudget is how many median inter-arrival intervals of history a baseline
// spans. It is a count of intervals, not samples or seconds, so depth scales
// with each series' own cadence.
const SampleBudget = 60

/*
Median returns the empirical median of series.
*/
func Median(series []float64) float64 {
	if len(series) == 0 {
		return 0
	}

	sorted := append([]float64(nil), series...)
	sort.Float64s(sorted)

	return stat.Quantile(0.5, stat.Empirical, sorted, nil)
}

/*
ScaleByMedian normalises sample by the median of baseline. With no baseline the
scale is undefined and callers must seed history before scoring.
*/
func ScaleByMedian(sample float64, baseline []float64) float64 {
	if len(baseline) == 0 {
		return 0
	}

	median := Median(baseline)

	if median <= 0 {
		return 0
	}

	return sample / median
}

/*
ScaleByMedianOrUnity normalises sample by the median of baseline. With no baseline
the sample is the mean of itself and scales to one.
*/
func ScaleByMedianOrUnity(sample float64, baseline []float64) float64 {
	if len(baseline) == 0 {
		return 1
	}

	return ScaleByMedian(sample, baseline)
}

/*
InvertedCompression scores spread tightening versus the baseline median spread.
*/
func InvertedCompression(spread float64, baseline []float64) float64 {
	median := Median(baseline)

	if median <= 0 || spread >= median {
		return 0
	}

	return (median - spread) / median
}

/*
WindowDepth returns how many of the most recent stamps to keep: those within
SampleBudget median inter-arrival intervals of the latest stamp.
*/
func WindowDepth(stamps []float64) int {
	if len(stamps) < 2 {
		return len(stamps)
	}

	gaps := make([]float64, 0, len(stamps)-1)

	for index := 1; index < len(stamps); index++ {
		if gap := stamps[index] - stamps[index-1]; gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	cadence := Median(gaps)

	if cadence <= 0 {
		if len(stamps) > SampleBudget {
			return SampleBudget
		}

		return len(stamps)
	}

	span := cadence * SampleBudget
	latest := stamps[len(stamps)-1]
	depth := 0

	for index := len(stamps) - 1; index >= 0 && latest-stamps[index] <= span; index-- {
		depth++
	}

	return depth
}

/*
Tail returns the last keep elements of series.
*/
func Tail(series []float64, keep int) []float64 {
	if keep <= 0 || keep >= len(series) {
		return series
	}

	return series[len(series)-keep:]
}

/*
NormalizeMasses divides each mass by the total. Non-finite and negative masses
are zeroed first. When total is zero masses are left unchanged.
*/
func NormalizeMasses(masses []float64) {
	total := 0.0

	for index := range masses {
		mass := masses[index]

		if math.IsNaN(mass) || math.IsInf(mass, 0) || mass < 0 {
			masses[index] = 0

			continue
		}

		total += mass
	}

	if total <= 0 {
		for index := range masses {
			masses[index] = 0
		}

		return
	}

	for index := range masses {
		masses[index] /= total
	}
}

/*
MaxMass returns the largest entry in masses, ignoring non-finite values.
*/
func MaxMass(masses []float64) float64 {
	best := 0.0

	for _, mass := range masses {
		if math.IsNaN(mass) || math.IsInf(mass, 0) || mass < 0 {
			continue
		}

		if mass > best {
			best = mass
		}
	}

	return best
}
