package fluid

import (
	"math"
)

const fluidDynamicsCap = 64
const minFluidDynamicsHistory = 4

type fluidDynamics struct {
	reynoldsHistory    []float64
	divergenceHistory  []float64
	sourceBalanceRatio []float64
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

func (dynamics *fluidDynamics) recordSourceBalance(addRate, executeRate float64) {
	if addRate <= 0 || executeRate <= 0 {
		return
	}

	activity := addRate + executeRate

	balanceRatio := 1 - math.Abs(addRate-executeRate)/activity

	dynamics.sourceBalanceRatio = appendRing(
		dynamics.sourceBalanceRatio,
		balanceRatio,
		fluidDynamicsCap,
	)
}

func (dynamics *fluidDynamics) icebergBalanceFloor() (float64, bool) {
	if len(dynamics.sourceBalanceRatio) >= minFluidDynamicsHistory {
		return sampleQuantile(0.75, dynamics.sourceBalanceRatio), true
	}

	return 0, false
}

func (dynamics *fluidDynamics) icebergScore(addRate, executeRate float64) float64 {
	if addRate <= 0 || executeRate <= 0 {
		return 0
	}

	activity := addRate + executeRate

	if activity <= 0 {
		return 0
	}

	balanceRatio := 1 - math.Abs(addRate-executeRate)/activity
	floor, ready := dynamics.icebergBalanceFloor()

	if !ready || balanceRatio < floor {
		return 0
	}

	return math.Min(addRate, executeRate)
}

func appendRing(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}
