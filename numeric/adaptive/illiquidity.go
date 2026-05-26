package adaptive

/*
IlliquidityScore returns a unitless score in (0, 1] for quoteVol strictly below
the cross-section median of quoteVol and peers. Zero when quoteVol is not illiquid.
*/
func IlliquidityScore(quoteVol float64, peers []float64) float64 {
	if quoteVol <= 0 || len(peers) < 2 {
		return 0
	}

	sample := append([]float64{quoteVol}, peers...)
	median := crossSectionMedian(sample)

	if median <= 0 || quoteVol >= median {
		return 0
	}

	return (median - quoteVol) / median
}
