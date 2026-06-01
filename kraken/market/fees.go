package market

import "fmt"

/*
TakerFeePercent returns the taker fee percent for quoteVolume30d on this pair's
Kraken tier schedule from GET /public/AssetPairs.
*/
func (pair *Pair) TakerFeePercent(quoteVolume30d float64) (float64, error) {
	if pair == nil {
		return 0, fmt.Errorf("kraken pair: nil receiver")
	}

	if len(pair.Fees) == 0 {
		return 0, fmt.Errorf("kraken pair %q: fees schedule missing", pair.Wsname)
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

	return feePct, nil
}

/*
MakerFeePercent returns the maker fee percent for quoteVolume30d on this pair's
Kraken tier schedule from GET /public/AssetPairs.
*/
func (pair *Pair) MakerFeePercent(quoteVolume30d float64) (float64, error) {
	if pair == nil {
		return 0, fmt.Errorf("kraken pair: nil receiver")
	}

	if len(pair.FeesMaker) == 0 {
		return 0, fmt.Errorf("kraken pair %q: fees_maker schedule missing", pair.Wsname)
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

	return feePct, nil
}
