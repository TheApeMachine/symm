package adaptive

import (
	"math"
)

/*
Gating passes signals when they exceed a dynamic statistical boundary
relative to real-time moments, and attenuates/filters otherwise.
Fulfills the zero-magic mandate: threshold dynamically evaluated from data.
*/
type Gating struct {
	Threshold Threshold

	engine WelfordEngine
}

func (gating *Gating) Step(number Number) Number {
	value := float64(number)
	mean, stdDev := gating.engine.Update(value)
	thresholdValue := gating.Threshold.Compute(value)

	distance := math.Abs(value - mean)

	if distance < thresholdValue && stdDev > 0 {
		return 0
	}

	return number
}
