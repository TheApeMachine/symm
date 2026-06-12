package numeric

import (
	"fmt"
	"math"
)

/*
BookLevelPrices extracts prices from parallel bid/ask level slices.
*/
func BookLevelPrices(bids, asks []float64) []float64 {
	prices := make([]float64, 0, len(bids)+len(asks))
	prices = append(prices, bids...)
	prices = append(prices, asks...)

	return prices
}

/*
InferBookTickSize estimates the exchange price increment from bid and ask ladders.
*/
func InferBookTickSize(bidPrices, askPrices []float64) float64 {
	tickSize := InferTickSizeFromPrices(bidPrices)

	if tickSize > 0 {
		return tickSize
	}

	return InferTickSizeFromPrices(askPrices)
}

/*
ResolveBookTickSize returns the ladder-inferred tick size, or fallback when inference fails.
*/
func ResolveBookTickSize(
	bidPrices, askPrices []float64,
	fallback float64,
) (float64, error) {
	tickSize := InferBookTickSize(bidPrices, askPrices)

	if tickSize > 0 {
		return tickSize, nil
	}

	if fallback > 0 {
		return fallback, nil
	}

	return 0, fmt.Errorf("numeric: book tick size unavailable")
}

/*
TouchBandCells returns the near-touch cell radius from spread and tick size.
*/
func TouchBandCells(spread, tickSize float64, halfWidth int) int {
	if spread <= 0 || tickSize <= 0 || halfWidth <= 0 {
		return 1
	}

	cells := int(math.Ceil(spread / tickSize))

	if cells < 1 {
		cells = 1
	}

	if cells > halfWidth {
		cells = halfWidth
	}

	return cells
}
