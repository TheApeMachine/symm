package leadlag

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
leadlagReading classifies the anchor relationship and returns shift evidence.
stallMargin is the unit headroom to the adaptive move floor when the anchor path
has not moved enough to lead.
*/
func leadlagReading(
	anchorMoved bool,
	stallMargin float64,
	corr float64,
	lagBars int,
) (perspectives.CategoryType, float64) {
	if !anchorMoved {
		if stallMargin <= 0 {
			return perspectives.CategoryAnchorStall, 0
		}

		return perspectives.CategoryAnchorStall, stallMargin
	}

	if lagBars > 0 {
		lagFraction := float64(lagBars) / float64(maxLagBars)

		if lagFraction >= minLagFraction {
			margin := lagFraction - minLagFraction
			span := 1 - minLagFraction

			if margin <= 0 || span <= 0 {
				return perspectives.CategoryInefficientLag, 0
			}

			return perspectives.CategoryInefficientLag, margin / span
		}

		margin := minLagFraction - lagFraction

		if margin <= 0 {
			return perspectives.CategorySynchronizedDrift, 0
		}

		return perspectives.CategorySynchronizedDrift, margin / minLagFraction
	}

	if corr < leadlagMinimumLagCorrelation {
		margin := leadlagMinimumLagCorrelation - corr

		if margin <= 0 {
			return perspectives.CategoryDecoupledMove, 0
		}

		return perspectives.CategoryDecoupledMove, margin / leadlagMinimumLagCorrelation
	}

	margin := corr - leadlagMinimumLagCorrelation
	span := 1 - leadlagMinimumLagCorrelation

	if margin <= 0 || span <= 0 {
		return perspectives.CategorySynchronizedDrift, 0
	}

	return perspectives.CategorySynchronizedDrift, margin / span
}
