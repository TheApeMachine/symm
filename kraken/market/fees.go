package market

const defaultTakerFeePct = 0.40

/*
TakerFeePercent returns the taker fee percent for quoteVolume30d on this pair's
Kraken tier schedule. When the schedule is missing, fallbackPct applies.
*/
func (pair *Pair) TakerFeePercent(quoteVolume30d, fallbackPct float64) float64 {
	if pair == nil || len(pair.Fees) == 0 {
		if fallbackPct > 0 {
			return fallbackPct
		}

		return defaultTakerFeePct
	}

	feePct := pair.Fees[0][1]

	for _, tier := range pair.Fees {
		if len(tier) < 2 {
			continue
		}

		if quoteVolume30d >= tier[0] {
			feePct = tier[1]
		}
	}

	return feePct
}

/*
MakerFeePercent returns the maker fee percent for quoteVolume30d.
*/
func (pair *Pair) MakerFeePercent(quoteVolume30d, fallbackPct float64) float64 {
	if pair == nil || len(pair.FeesMaker) == 0 {
		if fallbackPct > 0 {
			return fallbackPct
		}

		return defaultTakerFeePct
	}

	feePct := pair.FeesMaker[0][1]

	for _, tier := range pair.FeesMaker {
		if len(tier) < 2 {
			continue
		}

		if quoteVolume30d >= tier[0] {
			feePct = tier[1]
		}
	}

	return feePct
}
