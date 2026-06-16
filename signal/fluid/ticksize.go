package fluid

import (
	"fmt"
	"math"
	"sort"
)

/*
resolveBookTickSize derives the minimum positive price increment from book levels.
*/
func resolveBookTickSize(
	bidPrices []float64,
	askPrices []float64,
	fallback float64,
) (float64, error) {
	prices := append(append([]float64{}, bidPrices...), askPrices...)

	if len(prices) == 0 {
		if fallback > 0 {
			return fallback, nil
		}

		return 0, fmt.Errorf("fluid: book tick size requires prices or fallback")
	}

	sort.Float64s(prices)

	minStep := 0.0

	for index := 1; index < len(prices); index++ {
		step := prices[index] - prices[index-1]

		if step <= 0 {
			continue
		}

		if minStep == 0 || step < minStep {
			minStep = step
		}
	}

	if minStep > 0 {
		return minStep, nil
	}

	if fallback > 0 {
		return fallback, nil
	}

	return 0, fmt.Errorf("fluid: book tick size could not be resolved")
}

/*
touchBandCells counts lattice cells within the near-touch band around mid.
*/
func touchBandCells(spread, tickSize float64, halfWidth int) int {
	if tickSize <= 0 || spread <= 0 || halfWidth <= 0 {
		return 0
	}

	band := int(math.Ceil(spread / (2 * tickSize)))

	if band < 1 {
		return 1
	}

	if band > halfWidth {
		return halfWidth
	}

	return band
}
