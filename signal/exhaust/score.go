package exhaust

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
exitScoreLong estimates how urgently a long should be closed from book history.
*/
func exitScoreLong(history symbolHistory) (urgency float64, category types.CategoryType, evidence float64) {
	thinning := exitComponentScore(depthTrend(history.bidDepths))
	widen := exitComponentScore(spreadWiden(history.spreads))
	fade := exitComponentScore(pressureFade(history.pressures, 1))
	flip := imbalanceFlip(history.imbalances, 1)
	collapse := exitComponentScore(depthTrend(history.densities))
	thinning = types.AdjustSourceValue(types.SourceExhaustion, thinning)
	widen = types.AdjustSourceValue(types.SourceExhaustion, widen)
	fade = types.AdjustSourceValue(types.SourceExhaustion, fade)
	flip = types.AdjustSourceValue(types.SourceExhaustion, flip)
	collapse = types.AdjustSourceValue(types.SourceExhaustion, collapse)

	urgency = 0.30*thinning +
		0.20*widen +
		0.20*fade +
		0.15*flip +
		0.15*collapse

	if urgency <= 0 {
		return 0, types.CategoryTypeNone, 0
	}

	category, evidence = exhaustReading(thinning, widen, fade, flip)

	return urgency, category, evidence
}

/*
exitScoreShort estimates how urgently a short should be closed from book history.
*/
func exitScoreShort(history symbolHistory) (urgency float64, category types.CategoryType, evidence float64) {
	thinning := exitComponentScore(depthTrend(history.askDepths))
	widen := exitComponentScore(spreadWiden(history.spreads))
	fade := exitComponentScore(pressureFade(history.pressures, -1))
	flip := imbalanceFlip(history.imbalances, -1)
	collapse := exitComponentScore(depthTrend(history.densities))
	thinning = types.AdjustSourceValue(types.SourceExhaustion, thinning)
	widen = types.AdjustSourceValue(types.SourceExhaustion, widen)
	fade = types.AdjustSourceValue(types.SourceExhaustion, fade)
	flip = types.AdjustSourceValue(types.SourceExhaustion, flip)
	collapse = types.AdjustSourceValue(types.SourceExhaustion, collapse)

	urgency = 0.30*thinning +
		0.20*widen +
		0.20*fade +
		0.15*flip +
		0.15*collapse

	if urgency <= 0 {
		return 0, types.CategoryTypeNone, 0
	}

	category, evidence = exhaustReading(thinning, widen, fade, flip)

	return urgency, category, evidence
}

func exitComponentScore(value float64) float64 {
	return types.UnitMagnitudeMargin(value)
}
