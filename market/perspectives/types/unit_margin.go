package types

// UnitMarginFloor is the minimum non-zero unit competition or clarity margin
// signals emit before SNR scoring and ingest validation.
const UnitMarginFloor = 0.02

/*
UnitCompetitionMargin maps a non-negative margin and scale onto the unit band
without post-hoc capping: margin / (margin + scale). Large finite margins can
saturate to 1 under floating-point arithmetic.
*/
func UnitCompetitionMargin(margin, scale float64) float64 {
	if margin <= 0 || scale <= 0 {
		return 0
	}

	return margin / (margin + scale)
}

/*
UnitMagnitudeMargin maps a non-negative magnitude onto the unit band using a unit
scale of 1: magnitude / (magnitude + 1). Large finite magnitudes can saturate to
1 under floating-point arithmetic.
*/
func UnitMagnitudeMargin(magnitude float64) float64 {
	if magnitude <= 0 {
		return 0
	}

	return magnitude / (magnitude + 1)
}
