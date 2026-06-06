package exhaust

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	reasonBookThinning  = "book_thinning"
	reasonSpreadWiden   = "spread_widen"
	reasonPressureFade  = "pressure_fade"
	reasonImbalanceFlip = "imbalance_flip"
)

// uniformExhaustConfidence is the 1/N floor across the four exhaustion categories
// (mechanical collapse, fragile expansion, thermal exhaustion, active reversal): a
// selection with no margin over the runner-up is a uniform guess, never 0.
const uniformExhaustConfidence = 1.0 / 4.0

/*
exhaustReading picks the dominant exit mode and returns the confidence in that
selection: the mode's purity (margin over the runner-up) scaled by its intensity
(magnitude on the unit-fractional scale), so a faint lone mode is not falsely
certain. The aggregate urgency carries the SNR strength separately.
*/
func exhaustReading(
	thinning, widen, fade, flip float64,
) (types.CategoryType, float64) {
	reason := dominantExitReason(thinning, widen, fade, flip)
	winner := componentScore(thinning, widen, fade, flip, reason)
	runnerUp := 0.0

	for _, candidate := range []struct {
		name  string
		value float64
	}{
		{reasonBookThinning, thinning},
		{reasonSpreadWiden, widen},
		{reasonPressureFade, fade},
		{reasonImbalanceFlip, flip},
	} {
		if candidate.name == reason {
			continue
		}

		if candidate.value > runnerUp {
			runnerUp = candidate.value
		}
	}

	margin := winner - runnerUp

	if margin <= 0 {
		return exhaustCategory(reason), uniformExhaustConfidence
	}

	// Confidence is the dominant mode's PURITY (how cleanly it leads the runner-up)
	// scaled by its INTENSITY (how strongly it fires on the unit-fractional scale).
	// Purity alone saturates to 1 whenever a single mode is active — which is the
	// norm here, since spreadWiden/pressureFade/imbalanceFlip read exactly 0 when
	// their condition is unmet — so a 2% thinning flicker looked as certain as a
	// violent collapse. Folding in intensity makes a faint lone mode honestly
	// low-confidence without capping anything.
	dominance := margin / math.Max(winner, 1e-12)

	return exhaustCategory(reason), dominance * types.UnitMagnitudeMargin(winner)
}

func dominantExitReason(thinning, widen, fade, flip float64) string {
	best := thinning
	reason := reasonBookThinning

	if widen > best {
		best = widen
		reason = reasonSpreadWiden
	}

	if fade > best {
		best = fade
		reason = reasonPressureFade
	}

	if flip > best {
		reason = reasonImbalanceFlip
	}

	return reason
}

func exhaustCategory(reason string) types.CategoryType {
	switch reason {
	case reasonBookThinning:
		return types.CategoryMechanicalCollapse
	case reasonSpreadWiden:
		return types.CategoryFragileExpansion
	case reasonPressureFade:
		return types.CategoryThermalExhaustion
	case reasonImbalanceFlip:
		return types.CategoryActiveReversal
	default:
		return types.CategoryThermalExhaustion
	}
}

func componentScore(thinning, widen, fade, flip float64, reason string) float64 {
	switch reason {
	case reasonBookThinning:
		return thinning
	case reasonSpreadWiden:
		return widen
	case reasonPressureFade:
		return fade
	case reasonImbalanceFlip:
		return flip
	default:
		return 0
	}
}
