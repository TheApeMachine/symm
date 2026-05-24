package exhaust

/*
exitScoreLong estimates how urgently a long should be closed from book history.
*/
func exitScoreLong(history symbolHistory) (urgency float64, reason string) {
	thinning := depthTrend(history.bidDepths)
	widen := spreadWiden(history.spreads)
	fade := pressureFade(history.pressures, 1)
	flip := imbalanceFlip(history.imbalances, 1)

	urgency = 0.35*clamp01(thinning) +
		0.20*clamp01(widen) +
		0.25*clamp01(fade) +
		0.20*clamp01(flip)

	if urgency <= 0 {
		return 0, ""
	}

	reason = dominantExitReason(thinning, widen, fade, flip)

	return clamp01(urgency), reason
}

/*
exitScoreShort estimates how urgently a short should be closed from book history.
*/
func exitScoreShort(history symbolHistory) (urgency float64, reason string) {
	thinning := depthTrend(history.bidDepths)
	widen := spreadWiden(history.spreads)
	fade := pressureFade(history.pressures, -1)
	flip := imbalanceFlip(history.imbalances, -1)

	urgency = 0.35*clamp01(thinning) +
		0.20*clamp01(widen) +
		0.25*clamp01(fade) +
		0.20*clamp01(flip)

	if urgency <= 0 {
		return 0, ""
	}

	reason = dominantExitReason(thinning, widen, fade, flip)

	return clamp01(urgency), reason
}

func dominantExitReason(thinning, widen, fade, flip float64) string {
	best := thinning
	reason := "book_thinning"

	if widen > best {
		best = widen
		reason = "spread_widen"
	}

	if fade > best {
		best = fade
		reason = "pressure_fade"
	}

	if flip > best {
		reason = "imbalance_flip"
	}

	return reason
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
