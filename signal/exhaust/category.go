package exhaust

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	reasonBookThinning  = "book_thinning"
	reasonSpreadWiden   = "spread_widen"
	reasonPressureFade  = "pressure_fade"
	reasonImbalanceFlip = "imbalance_flip"
)

/*
exhaustReading picks the dominant exit mode and returns shift evidence — margin
over the runner-up component score.
*/
func exhaustReading(
	thinning, widen, fade, flip float64,
) (perspectives.CategoryType, float64) {
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
		return exhaustCategory(reason), 0
	}

	return exhaustCategory(reason), margin / math.Max(winner, 1e-12)
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

func exhaustCategory(reason string) perspectives.CategoryType {
	switch reason {
	case reasonBookThinning:
		return perspectives.CategoryMechanicalCollapse
	case reasonSpreadWiden:
		return perspectives.CategoryFragileExpansion
	case reasonPressureFade:
		return perspectives.CategoryThermalExhaustion
	case reasonImbalanceFlip:
		return perspectives.CategoryActiveReversal
	default:
		return perspectives.CategoryThermalExhaustion
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
