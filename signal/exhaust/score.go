package exhaust

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
exitScoreLong estimates how urgently a long should be closed from book history.
*/
func exitScoreLong(history symbolHistory) (urgency float64, category types.CategoryType, evidence float64) {
	thinning := depthTrend(history.bidDepths)
	widen := spreadWiden(history.spreads)
	fade := pressureFade(history.pressures, 1)
	flip := imbalanceFlip(history.imbalances, 1)
	collapse := depthTrend(history.densities)

	urgency = 0.30*clamp01(thinning) +
		0.20*clamp01(widen) +
		0.20*clamp01(fade) +
		0.15*clamp01(flip) +
		0.15*clamp01(collapse)

	if urgency <= 0 {
		return 0, types.CategoryTypeNone, 0
	}

	category, evidence = exhaustReading(thinning, widen, fade, flip)

	return clamp01(urgency), category, evidence
}

/*
exitScoreShort estimates how urgently a short should be closed from book history.
*/
func exitScoreShort(history symbolHistory) (urgency float64, category types.CategoryType, evidence float64) {
	thinning := depthTrend(history.askDepths)
	widen := spreadWiden(history.spreads)
	fade := pressureFade(history.pressures, -1)
	flip := imbalanceFlip(history.imbalances, -1)
	collapse := depthTrend(history.densities)

	urgency = 0.30*clamp01(thinning) +
		0.20*clamp01(widen) +
		0.20*clamp01(fade) +
		0.15*clamp01(flip) +
		0.15*clamp01(collapse)

	if urgency <= 0 {
		return 0, types.CategoryTypeNone, 0
	}

	category, evidence = exhaustReading(thinning, widen, fade, flip)

	return clamp01(urgency), category, evidence
}

func clamp01(value float64) float64 {
	if value <= 0 {
		return 0
	}

	if value >= 1 {
		return 1
	}

	return value
}
