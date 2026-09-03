package logic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
VolatilityBlend smoothly interpolates between low-volatility (branch A) and
high-volatility (branch B) paths based on real-time statistical variance
relative to an adaptive threshold boundary.
Fulfills the zero-magic principle: continuous sigmoid interpolation derived from
standardized volatility distance (stdDev - threshold) / stdDev.
*/
type VolatilityBlend struct {
	Window    adaptive.Window
	Threshold adaptive.Threshold

	engine adaptive.WelfordEngine
}

func (blend *VolatilityBlend) Route(number types.Number) (types.Number, types.Number, types.Number, types.Number) {
	_, stdDev := blend.engine.Update(float64(number))
	thresholdValue := blend.Threshold.Compute(stdDev)

	if thresholdValue <= 0 || stdDev <= 0 {
		return 1, 0, 0, 0
	}

	// Relative standardized distance from the threshold boundary in units of stdDev:
	// z = (stdDev - threshold) / stdDev
	z := (stdDev - thresholdValue) / stdDev

	// Logistic sigmoid transition: weightHigh = 1 / (1 + e^(-z))
	weightHigh := 1.0 / (1.0 + math.Exp(-z))
	weightLow := 1.0 - weightHigh

	return types.Number(weightLow), types.Number(weightHigh), 0, 0
}
