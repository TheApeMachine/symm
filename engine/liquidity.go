package engine

import "github.com/theapemachine/symm/numeric/adaptive"

const DefaultMinLiquidityPairs = 2

var belowMedianLiquidityGate = adaptive.NewBelowMedian()

/*
PassesBelowMedianLiquidity reports whether quoteVol is strictly below the
cross-section median of its own volume and positive peer quote volumes.
*/
func PassesBelowMedianLiquidity(
	quoteVol float64,
	quoteVolumes map[string]float64,
	symbol string,
	minPairs int,
) bool {
	if quoteVol <= 0 {
		return false
	}

	positive := positiveQuoteVolumes(quoteVolumes)

	if len(positive) < minPairs {
		return false
	}

	liquid, err := belowMedianLiquidityGate.Next(
		quoteVol,
		adaptive.PeerValues(positive, symbol)...,
	)

	if err != nil {
		return false
	}

	return liquid > 0
}

func positiveQuoteVolumes(volumes map[string]float64) map[string]float64 {
	positive := make(map[string]float64, len(volumes))

	for symbol, volume := range volumes {
		if volume > 0 {
			positive[symbol] = volume
		}
	}

	return positive
}

/*
QuoteVolumeMap builds a symbol → daily quote volume map from a lookup function.
*/
func QuoteVolumeMap(
	symbols []string,
	quoteVol func(symbol string) (float64, bool),
) map[string]float64 {
	quotes := make(map[string]float64, len(symbols))

	for _, symbol := range symbols {
		volume, ok := quoteVol(symbol)

		if ok && volume > 0 {
			quotes[symbol] = volume
		}
	}

	return quotes
}
