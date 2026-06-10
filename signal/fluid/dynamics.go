package fluid

import (
	"math"

	"github.com/theapemachine/symm/numeric"
)

const fluidDynamicsCap = 64

type fluidDynamics struct {
	reynoldsHistory   []float64
	divergenceHistory []float64
}

func (dynamics *fluidDynamics) recordReynolds(value float64) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	dynamics.reynoldsHistory = appendRing(dynamics.reynoldsHistory, value, fluidDynamicsCap)
}

func (dynamics *fluidDynamics) recordDivergence(value float64) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	dynamics.divergenceHistory = appendRing(dynamics.divergenceHistory, value, fluidDynamicsCap)
}

func (dynamics *fluidDynamics) laminarReynoldsCeiling(current float64) float64 {
	if len(dynamics.reynoldsHistory) >= 4 {
		sorted := numeric.CopySorted(dynamics.reynoldsHistory)

		return numeric.PercentileSorted(sorted, 0.25)
	}

	if current > 0 {
		return current
	}

	return 1
}

func (dynamics *fluidDynamics) turbulentReynoldsFloor(current float64) float64 {
	if len(dynamics.reynoldsHistory) >= 4 {
		sorted := numeric.CopySorted(dynamics.reynoldsHistory)

		return numeric.PercentileSorted(sorted, 0.75)
	}

	if current > 0 {
		return current * 2
	}

	return 2
}

func (dynamics *fluidDynamics) laminarDivergenceEdge() float64 {
	if len(dynamics.divergenceHistory) >= 4 {
		sorted := numeric.CopySorted(dynamics.divergenceHistory)

		return numeric.PercentileSorted(sorted, 0.25)
	}

	return 0
}

func appendRing(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}
