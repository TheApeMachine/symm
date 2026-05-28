package market

import "math"

const insufficientDepthImpactBPS = 150.0

/*
DepthFillVWAP walks order book levels until quoteNotional is filled.
When visible depth is insufficient, the remaining notional is crossed at an
adverse impact price beyond the worst visible level instead of returning zero.
*/
func DepthFillVWAP(
	levels []BookLevel,
	quoteNotional float64,
) float64 {
	return DepthFillVWAPSide(levels, quoteNotional, "buy")
}

/*
DepthFillVWAPSide returns a penalized VWAP for either side of the book.
*/
func DepthFillVWAPSide(
	levels []BookLevel,
	quoteNotional float64,
	side string,
) float64 {
	costSum, volumeSum, remaining := walkQuoteNotional(levels, quoteNotional)

	if volumeSum <= 0 {
		return 0
	}

	if remaining > 0 {
		impactPrice := adverseImpactPrice(levels, side)

		if impactPrice <= 0 {
			return 0
		}

		costSum += remaining
		volumeSum += remaining / impactPrice
	}

	return costSum / volumeSum
}

func adverseImpactPrice(levels []BookLevel, side string) float64 {
	worst := 0.0

	for _, level := range levels {
		if level.Price > 0 {
			worst = level.Price
		}
	}

	if worst <= 0 {
		return 0
	}

	impact := worst * insufficientDepthImpactBPS / 10000

	switch side {
	case "sell":
		return math.Max(worst-impact, math.SmallestNonzeroFloat64)
	default:
		return worst + impact
	}
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
