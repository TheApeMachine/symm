package signal

import "math"

/*
AnchorChange returns absolute and relative move from anchor to price.
*/
func AnchorChange(anchor, price float64) (move, precursor float64) {
	move = price - anchor

	if anchor > 0 {
		precursor = move / anchor
	}

	return move, precursor
}

/*
TouchSpread returns the price range observed across a touch window.
*/
func TouchSpread(prices []float64) (float64, bool) {
	if len(prices) < 2 {
		return 0, false
	}

	minPrice := prices[0]
	maxPrice := prices[0]

	for _, price := range prices[1:] {
		minPrice = math.Min(minPrice, price)
		maxPrice = math.Max(maxPrice, price)
	}

	spread := maxPrice - minPrice

	if spread <= 0 {
		return 0, false
	}

	return spread, true
}
