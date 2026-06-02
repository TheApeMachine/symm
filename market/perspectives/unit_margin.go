package perspectives

/*
UnitCompetitionMargin maps a non-negative margin and scale onto (0, 1) without
post-hoc capping: margin / (margin + scale).
*/
func UnitCompetitionMargin(margin, scale float64) float64 {
	if margin <= 0 || scale <= 0 {
		return 0
	}

	return margin / (margin + scale)
}

/*
UnitMagnitudeMargin maps a non-negative magnitude onto (0, 1) using a unit scale
of 1: magnitude / (magnitude + 1).
*/
func UnitMagnitudeMargin(magnitude float64) float64 {
	if magnitude <= 0 {
		return 0
	}

	return magnitude / (magnitude + 1)
}
