package statutil

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

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
sample is its own reference and scales to one.
*/
func ScaleByMedian(sample float64, baseline []float64) float64 {
	if len(baseline) == 0 {
		if sample <= 0 {
			return 0
		}

		return 1
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
		if sample <= 0 {
			return 0
		}

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
MedianCadence returns the median positive inter-arrival gap in stamps.
*/
func MedianCadence(stamps []float64) float64 {
	if len(stamps) < 2 {
		return 0
	}

	ordered := append([]float64(nil), stamps...)
	sort.Float64s(ordered)
	gaps := make([]float64, 0, len(ordered)-1)

	for index := 1; index < len(ordered); index++ {
		if gap := ordered[index] - ordered[index-1]; gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	return Median(gaps)
}

/*
SampleBudgetFromStamps derives how many median inter-arrival intervals the
observed stamp span covers.
*/
func SampleBudgetFromStamps(stamps []float64) int {
	if len(stamps) < 2 {
		return len(stamps)
	}

	ordered := append([]float64(nil), stamps...)
	sort.Float64s(ordered)
	cadence := MedianCadence(stamps)

	if cadence <= 0 {
		distinct, err := DistinctSpan(stamps)

		if err != nil || distinct < 2 {
			return len(stamps)
		}

		return int(distinct)
	}

	span := ordered[len(ordered)-1] - ordered[0]
	intervals := int(math.Ceil(span / cadence))

	if intervals < 2 {
		return 2
	}

	return intervals
}

/*
SampleBudgetFromCadence scales the interval budget from observed tick cadence.
*/
func SampleBudgetFromCadence(cadence float64) int {
	if cadence <= 0 {
		return 2
	}

	budget := int(math.Ceil(cadence + 1.0/cadence))

	if budget < 2 {
		return 2
	}

	return budget
}

/*
WindowDepth returns how many of the most recent stamps to keep: those within
a cadence-derived interval budget of the latest stamp.

The budget comes from SampleBudgetFromStamps, which is a scale-invariant ratio
(span / cadence). It must not be overridden by SampleBudgetFromCadence: that
takes a bare cadence value, which carries no span and is not scale-invariant, so
on nanosecond-scale stamps it explodes the window into billions of samples.
*/
func WindowDepth(stamps []float64) int {
	if len(stamps) < 2 {
		return len(stamps)
	}

	cadence := MedianCadence(stamps)

	if cadence <= 0 {
		return SampleBudgetFromStamps(stamps)
	}

	budget := SampleBudgetFromStamps(stamps)

	ordered := append([]float64(nil), stamps...)
	sort.Float64s(ordered)
	latest := ordered[len(ordered)-1]
	span := cadence * float64(budget)
	depth := 0

	for index := len(ordered) - 1; index >= 0 && latest-ordered[index] <= span; index-- {
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
