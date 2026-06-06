package leadlag

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
leadlagReading classifies the anchor relationship and returns two distinct
quantities: clarity — how decisively the reading clears the threshold that
separates its category from the neighbours — and strength — the magnitude of the
lead-lag phenomenon itself (the lag fraction, or the contemporaneous correlation),
which SNR scores against the symbol's own history. They answer different questions:
a barely-exploitable lag can be classified with high clarity, and a strong lag can
sit right on a boundary. stallMargin is the unit headroom to the adaptive move floor
when the anchor path has not moved enough to lead.
*/
func leadlagReading(
	anchorMoved bool,
	stallMargin float64,
	corr float64,
	lagBars int,
) (types.CategoryType, float64, float64) {
	if !anchorMoved {
		if stallMargin <= 0 {
			return types.CategoryAnchorStall, types.UnitMarginFloor, types.UnitMarginFloor
		}

		return types.CategoryAnchorStall, stallMargin, stallMargin
	}

	corrStrength := math.Max(0, math.Min(1, corr))

	if lagBars > 0 {
		lagFraction := float64(lagBars) / float64(maxLagBars)
		lagStrength := math.Min(1, lagFraction)

		if lagFraction >= minLagFraction {
			margin := lagFraction - minLagFraction
			span := 1 - minLagFraction

			if margin <= 0 || span <= 0 {
				return types.CategoryInefficientLag, types.UnitMarginFloor, lagStrength
			}

			return types.CategoryInefficientLag, margin / span, lagStrength
		}

		margin := minLagFraction - lagFraction

		if margin <= 0 {
			return types.CategorySynchronizedDrift, types.UnitMarginFloor, lagStrength
		}

		return types.CategorySynchronizedDrift, margin / minLagFraction, lagStrength
	}

	if corr < leadlagMinimumLagCorrelation {
		margin := leadlagMinimumLagCorrelation - corr
		span := 1 + leadlagMinimumLagCorrelation

		if margin <= 0 || span <= 0 {
			return types.CategoryDecoupledMove, types.UnitMarginFloor, corrStrength
		}

		return types.CategoryDecoupledMove, margin / span, corrStrength
	}

	margin := corr - leadlagMinimumLagCorrelation
	span := 1 - leadlagMinimumLagCorrelation

	if margin <= 0 || span <= 0 {
		return types.CategorySynchronizedDrift, types.UnitMarginFloor, corrStrength
	}

	return types.CategorySynchronizedDrift, margin / span, corrStrength
}
