package statutil

import (
	"math"
	"sort"
	"sync"

	"gonum.org/v1/gonum/stat"
)

var floatScratchPool = sync.Pool{
	New: func() any {
		scratch := make([]float64, 0, 256)

		return &scratch
	},
}

func borrowFloatScratch(size int) []float64 {
	scratchPtr := floatScratchPool.Get().(*[]float64)
	scratch := *scratchPtr

	if cap(scratch) < size {
		scratch = make([]float64, 0, size)
	}

	return scratch[:0]
}

func returnFloatScratch(scratch []float64) {
	if cap(scratch) > 1<<20 {
		return
	}

	scratch = scratch[:0]
	floatScratchPool.Put(&scratch)
}

func sortedFloatCopy(series []float64) []float64 {
	sorted := borrowFloatScratch(len(series))
	sorted = append(sorted, series...)
	sort.Float64s(sorted)

	return sorted
}

/*
Median returns the empirical median of series.
*/
func Median(series []float64) float64 {
	if len(series) == 0 {
		return 0
	}

	sorted := sortedFloatCopy(series)
	defer returnFloatScratch(sorted)

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

	ordered := sortedFloatCopy(stamps)
	defer returnFloatScratch(ordered)

	gaps := borrowFloatScratch(len(ordered) - 1)
	defer returnFloatScratch(gaps)

	for index := 1; index < len(ordered); index++ {
		if gap := ordered[index] - ordered[index-1]; gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	sort.Float64s(gaps)

	return stat.Quantile(0.5, stat.Empirical, gaps, nil)
}

/*
SampleBudgetFromStamps derives how many median inter-arrival intervals the
observed stamp span covers.
*/
func SampleBudgetFromStamps(stamps []float64) int {
	if len(stamps) < 2 {
		return len(stamps)
	}

	ordered := sortedFloatCopy(stamps)
	defer returnFloatScratch(ordered)

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
WindowDepth returns how many recent stamps to retain for a measurement history.
It is scale-invariant: cadence and median absolute jitter set the budget, and
the budget grows sublinearly with observed samples instead of retaining the
entire session lifetime.
*/
func WindowDepth(stamps []float64) int {
	if len(stamps) < 2 {
		return len(stamps)
	}

	ordered := sortedFloatCopy(stamps)
	defer returnFloatScratch(ordered)

	gaps := borrowFloatScratch(len(ordered) - 1)
	defer returnFloatScratch(gaps)

	for index := 1; index < len(ordered); index++ {
		if gap := ordered[index] - ordered[index-1]; gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 2
	}

	sort.Float64s(gaps)

	cadence := stat.Quantile(0.5, stat.Empirical, gaps, nil)

	if cadence <= 0 {
		return 2
	}

	deviations := borrowFloatScratch(len(gaps))
	defer returnFloatScratch(deviations)

	for _, gap := range gaps {
		deviations = append(deviations, math.Abs(gap-cadence))
	}

	sort.Float64s(deviations)

	jitter := stat.Quantile(0.5, stat.Empirical, deviations, nil) / cadence
	budget := int(math.Ceil(math.Log2(float64(len(gaps)+2))*(1+jitter))) + 1

	if budget < 2 {
		return 2
	}

	if budget > len(stamps) {
		return len(stamps)
	}

	return budget
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
