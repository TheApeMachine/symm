package correlation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLagSpacing         = nomagique.MustIntern("correlation/lag_spacing_nanos")
	SymbolMaximumLag         = nomagique.MustIntern("correlation/maximum_lag")
	SymbolBestLag            = nomagique.MustIntern("correlation/best_lag")
	SymbolBestLagCorrelation = nomagique.MustIntern("correlation/best_lag_correlation")
	SymbolContemporaneous    = nomagique.MustIntern("correlation/contemporaneous")
	SymbolSearchCount        = nomagique.MustIntern("correlation/search_count")
)

/*
Lag scans Hayashi correlation across an integral range of timestamp shifts.
Both paths are decoded once and reused for every candidate.
*/
func Lag(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	spacing, hasSpacing := input.Get(SymbolLagSpacing)
	maximumLagValue, hasMaximumLag := input.Get(SymbolMaximumLag)

	if !hasSpacing || spacing <= 0 || spacing != math.Trunc(spacing) ||
		!hasMaximumLag || maximumLagValue < 1 || maximumLagValue != math.Trunc(maximumLagValue) {
		return state, types.Frame{}, fmt.Errorf(
			"correlation: lag requires integral spacing and maximum lag",
		)
	}

	left, leftCount := pathPoints(&state)
	right, rightCount := pathPoints(&input)
	maximumLag := int(maximumLagValue)
	bestLag := 0
	bestCorrelation := 0.0
	bestMagnitude := 0.0
	contemporaneous := 0.0
	searchCount := 0

	for lag := -maximumLag; lag <= maximumLag; lag++ {
		correlation, _, _, _, _, ready := hayashiPoints(
			&left,
			leftCount,
			&right,
			rightCount,
			int64(float64(lag)*spacing),
		)

		if !ready {
			continue
		}

		if lag == 0 {
			contemporaneous = correlation
			continue
		}

		searchCount++
		magnitude := math.Abs(correlation)

		if magnitude <= bestMagnitude {
			continue
		}

		bestLag = lag
		bestCorrelation = correlation
		bestMagnitude = magnitude
	}

	output := input
	output.Put(SymbolBestLag, float64(bestLag))
	output.Put(SymbolBestLagCorrelation, bestCorrelation)
	output.Put(SymbolContemporaneous, contemporaneous)
	output.Put(SymbolSearchCount, float64(searchCount))
	output.Put(SymbolReady, truth(searchCount > 0))

	return state, output, nil
}
