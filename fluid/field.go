package fluid

import "math"

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

/*
burgersShock estimates shock strength from velocity nonlinearity over viscosity.
*/
func burgersShock(current, prior fieldSample) float64 {
	if current.viscosity <= 0 {
		return 0
	}

	velocityJump := math.Abs(current.velocity - prior.velocity)

	return math.Abs(current.velocity) * velocityJump / current.viscosity
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

	lower, upper := quartiles(ratios)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return maxFloat(ratios)
}

func medianAbs(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	magnitudes := make([]float64, len(values))

	for index, value := range values {
		magnitudes[index] = math.Abs(value)
	}

	return percentileSorted(copySorted(magnitudes), 0.5)
}

func quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sorted := copySorted(values)

	return percentileSorted(sorted, 0.25), percentileSorted(sorted, 0.75)
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

func copySorted(values []float64) []float64 {
	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return cp
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak
}

func sortFloats(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}

func crossSectionMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	return percentileSorted(copySorted(values), 0.5)
}
