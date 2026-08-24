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
		leftReturns, leftReturnCount, leftVariance := seriesReturns(&left, leftCount)
		rightReturns, rightReturnCount, rightVariance := seriesReturns(&right, rightCount)

		maximumLag := int(maximumLagValue)
		bestLag := 0
		bestCorrelation := 0.0
		bestMagnitude := 0.0
		contemporaneous := 0.0
		searchCount := 0

		// Retain each scanned lag's correlation and support indexed relative to
		// maximumLag so the best-lag support and both neighbors are read back
		// from this scan instead of recomputing hayashiPoints afterward.
		lagCount := 2*maximumLag + 1
		lagCovariances := make([]float64, lagCount)
		lagSupports := make([]float64, lagCount)
		lagReady := make([]bool, lagCount)

		for lag := -maximumLag; lag <= maximumLag; lag++ {
			correlation, covariance, _, _, support, ready := hayashiPoints(
				&leftReturns,
				leftReturnCount,
				leftVariance,
				&rightReturns,
				rightReturnCount,
				rightVariance,
				int64(float64(lag)*spacing),
			)

			index := lag + maximumLag
			lagCovariances[index] = covariance
			lagSupports[index] = float64(support)
			lagReady[index] = ready

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
			bestIndex := bestLag + maximumLag
			input.Put(SymbolBestLagSupport, lagSupports[bestIndex])

			if bestLag-1 >= -maximumLag {
				if neighbor := lagReady[bestIndex-1]; neighbor {
					input.Put(SymbolNeighborLow, lagCovariances[bestIndex-1])
				}
			}

			if bestLag+1 <= maximumLag {
				if neighbor := lagReady[bestIndex+1]; neighbor {
					input.Put(SymbolNeighborHigh, lagCovariances[bestIndex+1])
				}
			}
		}

		return input
	}
}
