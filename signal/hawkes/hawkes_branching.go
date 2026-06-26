package hawkes

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/statutil"
)

func stabilityCeiling(branching float64, gapCount int) float64 {
	evidence := 1.0

	if gapCount > 0 {
		evidence = 1 - 1/float64(gapCount+1)
	}

	return math.Min(1, branching+evidence*(1-branching))
}

func tailQuantile(gapCount int, branching float64) float64 {
	if gapCount <= 0 {
		return branching
	}

	sampleRank := 1 - 1/float64(gapCount)

	return sampleRank * (0.5 + branching/2)
}

/*
criticalRadiusCap derives the stability ceiling from arrival clustering history.
*/
func criticalRadiusCap(stamps []float64, branching float64) float64 {
	gaps := interArrivalGaps(stamps)
	ceiling := stabilityCeiling(branching, len(gaps))

	if branching <= 0 {
		return ceiling
	}

	if len(gaps) == 0 {
		return math.Min(branching, ceiling)
	}

	medianGap := statutil.Median(gaps)
	tailGap, quantileErr := statutil.Quantile(tailQuantile(len(gaps), branching), gaps)

	if quantileErr != nil || medianGap <= 0 {
		return math.Min(branching, ceiling)
	}

	clusterTightness := tailGap / medianGap
	tightnessMargin := math.Max(0, 1-clusterTightness)
	cap := branching * (1 + tightnessMargin/(1+tightnessMargin))

	return math.Min(math.Max(cap, branching), ceiling)
}

func interArrivalGaps(stamps []float64) []float64 {
	if len(stamps) < 2 {
		return nil
	}

	gaps := make([]float64, 0, len(stamps)-1)

	for index := 1; index < len(stamps); index++ {
		gap := stamps[index] - stamps[index-1]

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

/*
intensityOf is the arrival rate of a side in per-SECOND units: its event count
divided by the wall-clock span the events cover. Stamps are nanosecond epochs
(datapoint.Timestamp), so the raw span is divided by 1e9 before forming the rate;
without that an unnormalised lambda would read ~1e9x too small (per-nanosecond,
not per-second). A single event has no span and reads as one event over a unit
(one-second) interval. The series is assumed time-ordered as written by Measure.
*/
func intensityOf(stamps []float64) float64 {
	if len(stamps) == 0 {
		return 0
	}

	if len(stamps) == 1 {
		return 1
	}

	spanSeconds := (stamps[len(stamps)-1] - stamps[0]) / 1e9

	if spanSeconds <= 0 {
		return float64(len(stamps))
	}

	return float64(len(stamps)) / spanSeconds
}

/*
branchingRatio reads the endogenous feedback factor from arrival clustering: the
fraction of consecutive inter-arrival gaps that fall below the series' own median
gap. A self-exciting flow bunches its gaps (most below median), an exogenous
Poisson flow scatters them evenly (about half below). Fewer than three arrivals
carry no cadence and read as zero feedback.
*/
func branchingRatio(stamps []float64) float64 {
	if len(stamps) < 3 {
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

	if len(gaps) == 0 {
		return 0
	}

	median := statutil.Median(gaps)
	below := 0

	for _, gap := range gaps {
		if gap < median {
			below++
		}
	}

	return float64(below) / float64(len(gaps))
}
