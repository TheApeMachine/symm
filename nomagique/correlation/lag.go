package correlation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLagSpacing         = types.MustIntern("correlation/lag_spacing_nanos")
	SymbolMaximumLag         = types.MustIntern("correlation/maximum_lag")
	SymbolBestLag            = types.MustIntern("correlation/best_lag")
	SymbolBestLagCorrelation = types.MustIntern("correlation/best_lag_correlation")
	SymbolContemporaneous    = types.MustIntern("correlation/contemporaneous")
	SymbolSearchCount        = types.MustIntern("correlation/search_count")
	SymbolBestLagSupport     = types.MustIntern("correlation/best_lag_support")
	SymbolNeighborLow        = types.MustIntern("correlation/neighbor_low")
	SymbolNeighborHigh       = types.MustIntern("correlation/neighbor_high")
)

/*
Lag returns the primitive that scans Hayashi correlation between the two series
across an integral range of timestamp shifts. Both paths are decoded once and
reused for every candidate.
*/
func Lag(leftPrefix string, rightPrefix string) types.Primitive {
	leftSeries := temporal.NewSeries(leftPrefix)
	rightSeries := temporal.NewSeries(rightPrefix)

	return func(input types.Frame) types.Frame {
		spacing, hasSpacing := input.Get(SymbolLagSpacing)
		maximumLagValue, hasMaximumLag := input.Get(SymbolMaximumLag)

		if !hasSpacing || spacing <= 0 || spacing != math.Trunc(spacing) ||
			!hasMaximumLag || maximumLagValue < 1 || maximumLagValue != math.Trunc(maximumLagValue) {
			input.Err = fmt.Errorf(
				"correlation: lag requires integral spacing and maximum lag",
			)

			return input
		}

		left, leftCount := seriesPoints(leftSeries, &input)
		right, rightCount := seriesPoints(rightSeries, &input)
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

		input.Put(SymbolBestLag, float64(bestLag))
		input.Put(SymbolBestLagCorrelation, bestCorrelation)
		input.Put(SymbolContemporaneous, contemporaneous)
		input.Put(SymbolSearchCount, float64(searchCount))
		input.Put(SymbolReady, truth(searchCount > 0))

		if searchCount > 0 {
			neighborShift := int64(spacing)
			shift := int64(float64(bestLag) * spacing)
			_, _, _, _, support, _ := hayashiPoints(
				&left, leftCount, &right, rightCount, shift,
			)
			input.Put(SymbolBestLagSupport, float64(support))

			if bestLag-1 >= -maximumLag {
				_, neighborLow, _, _, _, hasLow := hayashiPoints(
					&left, leftCount, &right, rightCount, shift-neighborShift,
				)

				if hasLow {
					input.Put(SymbolNeighborLow, neighborLow)
				}
			}

			if bestLag+1 <= maximumLag {
				_, neighborHigh, _, _, _, hasHigh := hayashiPoints(
					&left, leftCount, &right, rightCount, shift+neighborShift,
				)

				if hasHigh {
					input.Put(SymbolNeighborHigh, neighborHigh)
				}
			}
		}

		return input
	}
}
