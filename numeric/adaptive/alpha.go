package adaptive

/*
AlphaFromSurprise maps a cross-section surprise index to an EWM blending rate.
Values at or below 1 retain alphaMin; values at or above 2 reach alphaMax.
*/
func AlphaFromSurprise(surpriseIndex, alphaMin, alphaMax float64) float64 {
	if alphaMax <= alphaMin {
		return alphaMin
	}

	if alphaMin <= 0 {
		alphaMin = 0.001
	}

	excess := surpriseIndex - 1

	if excess <= 0 {
		return alphaMin
	}

	if excess >= 1 {
		return alphaMax
	}

	return alphaMin + (alphaMax-alphaMin)*excess
}
