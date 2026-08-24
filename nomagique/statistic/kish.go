package statistic

/*
EffectiveSampleSize returns the Kish effective sample size for a weight
vector:

	N_eff = (sum w)² / sum(w²)

It is the normative support contract for estimator maturity. A zero-length
weight vector has effective sample size zero.
*/
func EffectiveSampleSize(weights []float64) float64 {
	sum := 0.0
	sumSquares := 0.0

	for _, weight := range weights {
		sum += weight
		sumSquares += weight * weight
	}

	if sumSquares == 0 {
		return 0
	}

	return sum * sum / sumSquares
}

/*
KishMaturity maps effective sample support to the normative maturity measure:

	Maturity = 0             when N_eff <= 1
	Maturity = 1 - 1/N_eff   otherwise

Maturity measures effective support. It is not a readiness threshold and it
does not override identification.
*/
func KishMaturity(weights []float64) float64 {
	effective := EffectiveSampleSize(weights)

	if effective <= 1 {
		return 0
	}

	return 1 - 1/effective
}
