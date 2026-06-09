package causal

import "math"

/*
regimeTracker applies temporal hysteresis so noisy condition/contagion trips do
not whip the causal model between flow and liquidity panic roles.
*/
type regimeTracker struct {
	inverted      bool
	pendingPanic  int
	pendingNormal int
}

func deriveRegimeHysteresisSamples(historyLen int) int {
	if historyLen <= 0 {
		return 2
	}

	samples := int(math.Ceil(math.Sqrt(float64(historyLen))))

	if samples < 2 {
		return 2
	}

	return samples
}

func (tracker *regimeTracker) apply(rawInverted bool, hysteresis int) bool {
	if hysteresis <= 0 {
		hysteresis = 1
	}

	if rawInverted {
		tracker.pendingPanic++
		tracker.pendingNormal = 0

		if tracker.pendingPanic >= hysteresis {
			tracker.inverted = true
		}
	} else {
		tracker.pendingNormal++
		tracker.pendingPanic = 0

		if tracker.pendingNormal >= hysteresis {
			tracker.inverted = false
		}
	}

	return tracker.inverted
}
