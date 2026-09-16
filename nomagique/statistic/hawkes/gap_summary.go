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
func (gapSummary *gapSummary) reset(marked []markedEvent) {
	gapSummary.sorted = gapSummary.sorted[:0]

	for index := 1; index < len(marked); index++ {
		gap := marked[index].atSec - marked[index-1].atSec

		if gap > 0 {
			gapSummary.sorted = append(gapSummary.sorted, gap)
		}
	}

	sort.Float64s(gapSummary.sorted)
}

func (gapSummary gapSummary) finite() bool {
	for _, value := range gapSummary.sorted {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

/*
median returns the middle inter-arrival gap.
*/
func (gapSummary gapSummary) median() (float64, bool) {
	if len(gapSummary.sorted) == 0 || !gapSummary.finite() {
		return 0, false
	}

	middle := len(gapSummary.sorted) / 2

	if len(gapSummary.sorted)%2 == 0 {
		return (gapSummary.sorted[middle-1] + gapSummary.sorted[middle]) / 2, true
	}

	return gapSummary.sorted[middle], true
}

/*
quartiles returns the lower and upper quartile inter-arrival gaps.
*/
func (gapSummary gapSummary) quartiles() (float64, float64, error) {
	if len(gapSummary.sorted) == 0 {
		return 0, 0, fmt.Errorf("hawkes grid: quartiles require values")
	}

	if !gapSummary.finite() {
		return 0, 0, fmt.Errorf("hawkes grid: quartiles sample is non-finite")
	}

	lower := stat.Quantile(0.25, stat.LinInterp, gapSummary.sorted, nil)
	upper := stat.Quantile(0.75, stat.LinInterp, gapSummary.sorted, nil)

	return lower, upper, nil
}
