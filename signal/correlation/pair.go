package correlation

/*
pairRevision is one peer-side covariance and cohort-sum update staged around a
variance change so old and new correlations use the correct denominators.
*/
type pairRevision struct {
	peerSymbol string
	peer       *symbolState
	delta      float64
	oldRho     float64
	oldOK      bool
	seed       bool
}

/*
exactVariance rebuilds Hayashi variance from retained log-prices so streaming
± ret² updates cannot leave residual float noise in the denominator.
*/
func exactVariance(state *symbolState) float64 {
	if len(state.samples) < 2 || len(state.logPrices) != len(state.samples) {
		return 0
	}

	sum := 0.0

	for index := 0; index < len(state.logPrices)-1; index++ {
		if !hayashiIntervalOK(state.samples[index], state.samples[index+1]) {
			continue
		}

		ret := state.logPrices[index+1] - state.logPrices[index]
		sum += ret * ret
	}

	return sum
}
