package leadlag

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
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
) (perspectives.CategoryType, float64, float64) {
	if !anchorMoved {
		// A stalled anchor is the absence of a lead-lag phenomenon: clarity is the
		// headroom to the move floor, strength is zero.
		if stallMargin <= 0 {
			return perspectives.CategoryAnchorStall, 0, 0
		}

		return perspectives.CategoryAnchorStall, stallMargin, 0
	}

	corrStrength := math.Max(0, math.Min(1, corr))

	if lagBars > 0 {
		lagFraction := float64(lagBars) / float64(maxLagBars)
		lagStrength := math.Min(1, lagFraction)

		if lagFraction >= minLagFraction {
			margin := lagFraction - minLagFraction
			span := 1 - minLagFraction

			if margin <= 0 || span <= 0 {
				return perspectives.CategoryInefficientLag, 0, lagStrength
			}

			return perspectives.CategoryInefficientLag, margin / span, lagStrength
		}

		margin := minLagFraction - lagFraction

		if margin <= 0 {
			return perspectives.CategorySynchronizedDrift, 0, lagStrength
		}

		return perspectives.CategorySynchronizedDrift, margin / minLagFraction, lagStrength
	}

	if corr < leadlagMinimumLagCorrelation {
		margin := leadlagMinimumLagCorrelation - corr

		if margin <= 0 {
			return perspectives.CategoryDecoupledMove, 0, corrStrength
		}

		return perspectives.CategoryDecoupledMove, margin / leadlagMinimumLagCorrelation, corrStrength
	}

	margin := corr - leadlagMinimumLagCorrelation
	span := 1 - leadlagMinimumLagCorrelation

	if margin <= 0 || span <= 0 {
		return perspectives.CategorySynchronizedDrift, 0, corrStrength
	}

	return perspectives.CategorySynchronizedDrift, margin / span, corrStrength
}
