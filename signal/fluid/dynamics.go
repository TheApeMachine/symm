package fluid

import (
	"math"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat"
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
		return float64(statistic.NewQuantile(0.25, stat.LinInterp, nil).Observe(nomagique.Numbers(dynamics.reynoldsHistory...)...))
	}

	if current > 0 {
		return current
	}

	return 1
}

func (dynamics *fluidDynamics) turbulentReynoldsFloor(current float64) float64 {
	if len(dynamics.reynoldsHistory) >= 4 {
		return float64(statistic.NewQuantile(0.75, stat.LinInterp, nil).Observe(nomagique.Numbers(dynamics.reynoldsHistory...)...))
	}

	if current > 0 {
		return current * 2
	}

	return 2
}

func (dynamics *fluidDynamics) laminarDivergenceEdge() float64 {
	if len(dynamics.divergenceHistory) >= 4 {
		return float64(statistic.NewQuantile(0.25, stat.LinInterp, nil).Observe(nomagique.Numbers(dynamics.divergenceHistory...)...))
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
