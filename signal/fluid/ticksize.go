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
	minStep := 0.0

	for _, sideStep := range []float64{
		minStepFromPrices(bidPrices),
		minStepFromPrices(askPrices),
	} {
		if sideStep <= 0 {
			continue
		}

		if minStep == 0 || sideStep < minStep {
			minStep = sideStep
		}
	}

	if minStep > 0 {
		return minStep, nil
	}

	if fallback > 0 {
		return fallback, nil
	}

	if len(bidPrices) == 0 && len(askPrices) == 0 {
		return 0, fmt.Errorf("fluid: book tick size requires prices or fallback")
	}

	return 0, fmt.Errorf("fluid: book tick size could not be resolved")
}

func minStepFromPrices(prices []float64) float64 {
	if len(prices) < 2 {
		return 0
	}

	sorted := append([]float64(nil), prices...)
	sort.Float64s(sorted)

	minStep := 0.0

	for index := 1; index < len(sorted); index++ {
		step := sorted[index] - sorted[index-1]

		if step <= 0 {
			continue
		}

		if minStep == 0 || step < minStep {
			minStep = step
		}
	}

	return minStep
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
