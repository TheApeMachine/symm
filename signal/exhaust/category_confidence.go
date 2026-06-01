package exhaust

import "math"

/*
exitReasonConfidence returns how decisively one exit mode beat the runner-up.
*/
func exitReasonConfidence(thinning, widen, fade, flip float64, reason string) float64 {
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
		return 0
	}

	return margin / math.Max(winner, 1e-12)
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
