package fluid

import (
	"math"

	"github.com/theapemachine/symm/stats"
)

const minFieldHistory = 6

/*
fieldSample holds one discretized LOB fluid observation.
*/
type fieldSample struct {
	density   float64
	velocity  float64
	viscosity float64
	flow      float64
}

/*
continuitySource estimates source-sink mass from inflow against density change.
*/
func continuitySource(current, prior fieldSample) float64 {
	if current.density <= 0 || prior.density <= 0 || current.flow <= 0 {
		return 0
	}

	densityChange := current.density - prior.density
	expectedChange := current.flow / math.Max(current.density, prior.density)

	return densityChange - expectedChange
}

const minViscosityEpsilon = 1e-9

/*
burgersShock estimates shock strength from velocity nonlinearity over viscosity.
Near-zero viscosity uses an epsilon floor so thin-book events spike instead of flatlining.
*/
func burgersShock(current, prior fieldSample) float64 {
	viscosity := math.Max(current.viscosity, minViscosityEpsilon)
	velocityJump := math.Abs(current.velocity - prior.velocity)

	return math.Abs(current.velocity) * velocityJump / viscosity
}

func fieldConfidence(source, shock, buyPressure float64, quiet bool) float64 {
	if buyPressure <= 0 {
		return 0
	}

	buySide := (buyPressure + 1) / 2

	if buySide > 1 {
		buySide = 1
	}

	confidence := 0.0

	if quiet && source > 0 {
		confidence += source * buySide
	}

	if shock > 0 {
		confidence += shock * buySide
	}

	return confidence
}

func quietVelocity(velocities []float64, currentVelocity float64) bool {
	if len(velocities) < minFieldHistory {
		return false
	}

	medianSpeed := medianAbs(velocities)

	if medianSpeed <= 0 {
		return math.Abs(currentVelocity) <= 0
	}

	return math.Abs(currentVelocity) <= medianSpeed
}

func ratioFence(ratios []float64) float64 {
	if len(ratios) == 0 {
		return 0
	}

	lower, upper := stats.Quartiles(ratios)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return stats.Max(ratios)
}

func medianAbs(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	magnitudes := make([]float64, len(values))

	for index, value := range values {
		magnitudes[index] = math.Abs(value)
	}

	return stats.PercentileSorted(stats.CopySorted(magnitudes), 0.5)
}

func crossSectionMedian(values []float64) float64 {
	return stats.Median(values)
}
