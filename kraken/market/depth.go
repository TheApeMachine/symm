package market

import "math"

/*
DepthFillVWAP walks order book levels until quoteNotional is filled.
Returns zero when levels cannot cover the order.
*/
func DepthFillVWAP(
	levels []BookLevel,
	quoteNotional float64,
) float64 {
	costSum, volumeSum, remaining := walkQuoteNotional(levels, quoteNotional)

	if remaining > 0 || volumeSum <= 0 {
		return 0
	}

	return costSum / volumeSum
}

/*
DepthSlope estimates cumulative volume per unit price step across levels.
Higher slope means deeper liquidity near the touch.
*/
func DepthSlope(levels []BookLevel) float64 {
	if len(levels) < 2 {
		return 0
	}

	cumulative := 0.0
	priceSpan := math.Abs(levels[len(levels)-1].Price - levels[0].Price)

	for _, level := range levels {
		if level.Volume > 0 {
			cumulative += level.Volume
		}
	}

	if priceSpan <= 0 || cumulative <= 0 {
		return 0
	}

	return cumulative / priceSpan
}
