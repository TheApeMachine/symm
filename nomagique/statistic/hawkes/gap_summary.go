package hawkes

import (
	"fmt"
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

/*
gapSummary holds inter-arrival gap statistics (seconds) for one arrival
stream: the data-derived scale every optimizer bound and multi-start seed in
this package is stated in.
*/
type gapSummary struct {
	sorted []float64
}

func newGapSummaryFromGaps(gaps []float64) gapSummary {
	sorted := append([]float64(nil), gaps...)
	sort.Float64s(sorted)

	return gapSummary{sorted: sorted}
}

/*
reset rebuilds the summary from marked events into the caller-owned backing
array, avoiding an allocation on every workspace reuse.
*/
func (summary *gapSummary) reset(marked []markedEvent) {
	summary.sorted = summary.sorted[:0]

	for index := 1; index < len(marked); index++ {
		gap := marked[index].atSec - marked[index-1].atSec

		if gap > 0 {
			summary.sorted = append(summary.sorted, gap)
		}
	}

	sort.Float64s(summary.sorted)
}

func (summary gapSummary) finite() bool {
	for _, value := range summary.sorted {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

/*
median returns the middle inter-arrival gap.
*/
func (summary gapSummary) median() (float64, bool) {
	if len(summary.sorted) == 0 || !summary.finite() {
		return 0, false
	}

	middle := len(summary.sorted) / 2

	if len(summary.sorted)%2 == 0 {
		return (summary.sorted[middle-1] + summary.sorted[middle]) / 2, true
	}

	return summary.sorted[middle], true
}

/*
quartiles returns the lower and upper quartile inter-arrival gaps.
*/
func (summary gapSummary) quartiles() (float64, float64, error) {
	if len(summary.sorted) == 0 {
		return 0, 0, fmt.Errorf("hawkes grid: quartiles require values")
	}

	if !summary.finite() {
		return 0, 0, fmt.Errorf("hawkes grid: quartiles sample is non-finite")
	}

	lower := stat.Quantile(0.25, stat.LinInterp, summary.sorted, nil)
	upper := stat.Quantile(0.75, stat.LinInterp, summary.sorted, nil)

	return lower, upper, nil
}
