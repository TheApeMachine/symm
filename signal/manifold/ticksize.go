package manifold

import (
	"fmt"
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
		return 0, fmt.Errorf("manifold: book tick size requires prices or fallback")
	}

	return 0, fmt.Errorf("manifold: book tick size could not be resolved")
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
