package leadlag

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
categoryConfidence returns how decisively the lead-lag category wins over its
neighbors — not how large the lag correlation strength is.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	anchorChange, corr float64,
	lagBars int,
) float64 {
	switch category {
	case perspectives.CategoryAnchorStall:
		return anchorStallConfidence(anchorChange)
	case perspectives.CategoryInefficientLag:
		return inefficientLagConfidence(lagBars)
	case perspectives.CategoryDecoupledMove:
		return decoupledConfidence(corr)
	case perspectives.CategorySynchronizedDrift:
		return synchronizedConfidence(corr, lagBars)
	default:
		return 0
	}
}

func anchorStallConfidence(anchorChange float64) float64 {
	margin := minAnchorMove - anchorChange

	if margin <= 0 {
		return 0
	}

	return margin / minAnchorMove
}

func inefficientLagConfidence(lagBars int) float64 {
	lagFraction := float64(lagBars) / float64(maxLagBars)
	margin := lagFraction - minLagFraction

	if margin <= 0 {
		return 0
	}

	span := 1 - minLagFraction

	if span <= 0 {
		return 0
	}

	return margin / span
}

func decoupledConfidence(corr float64) float64 {
	margin := leadlagMinimumLagCorrelation - corr

	if margin <= 0 {
		return 0
	}

	return margin / leadlagMinimumLagCorrelation
}

func synchronizedConfidence(corr float64, lagBars int) float64 {
	if lagBars > 0 {
		lagFraction := float64(lagBars) / float64(maxLagBars)
		margin := minLagFraction - lagFraction

		if margin <= 0 {
			return 0
		}

		return margin / minLagFraction
	}

	margin := corr - leadlagMinimumLagCorrelation

	if margin <= 0 {
		return 0
	}

	span := 1 - leadlagMinimumLagCorrelation

	if span <= 0 {
		return 0
	}

	return margin / span
}
