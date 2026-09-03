package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
AdaptiveZScore (Tier 4 Equation):
Composes Welford moments with causal centering, scaling, and maturity.
Exposes zero-cost accessors for multi-metric projection.
Zero heap allocations, zero magic numbers.
*/
type AdaptiveZScore struct {
	welford adaptive.WelfordEngine

	hasPrior       bool
	lastZ          types.Scalar
	lastDivergence types.Scalar
	lastBaseline   types.Scalar
	lastRatio      types.Scalar
	lastMaturity   types.Scalar
}

func (eq *AdaptiveZScore) Step(x types.Scalar) types.Scalar {
	priorCount := eq.welford.Count()
	priorMean := eq.welford.Mean()
	priorDisp := eq.welford.Dispersion()

	// 1. Advance the underlying Welford primitive with log-observation for log-scale processes
	val := float64(x)
	var logVal float64

	if val > 0 {
		logVal = math.Log(val)
	}

	eq.welford.Update(logVal)

	// 2. Compute emergent dynamics relative to prior moments
	if priorCount == 0 {
		eq.lastBaseline = x
		eq.lastRatio = 1.0
		eq.lastDivergence = 0.0
		eq.lastZ = 0.0
		eq.lastMaturity = 0.0
		eq.hasPrior = false

		return 0.0
	}

	eq.hasPrior = true
	eq.lastBaseline = types.Scalar(math.Exp(priorMean))

	if eq.lastBaseline > 0 {
		eq.lastRatio = x / eq.lastBaseline
	} else {
		eq.lastRatio = 1.0
	}

	eq.lastDivergence = types.Scalar(logVal - priorMean)

	disp := priorDisp

	if disp <= 0 {
		disp = math.Abs(float64(eq.lastDivergence))
	}

	if disp > 0 {
		eq.lastZ = eq.lastDivergence / types.Scalar(disp)
	} else {
		eq.lastZ = 0.0
	}

	// 3. Exact Kish / Bayesian maturity: 1 - 1/(N + 1)
	eq.lastMaturity = types.Scalar(1.0 - 1.0/(priorCount+1.0))

	return eq.lastZ
}

// Accessors for multi-metric projection (0 allocs)
func (eq *AdaptiveZScore) HasPrior() bool           { return eq.hasPrior }
func (eq *AdaptiveZScore) ZScore() types.Scalar     { return eq.lastZ }
func (eq *AdaptiveZScore) Baseline() types.Scalar   { return eq.lastBaseline }
func (eq *AdaptiveZScore) Ratio() types.Scalar      { return eq.lastRatio }
func (eq *AdaptiveZScore) Divergence() types.Scalar { return eq.lastDivergence }
func (eq *AdaptiveZScore) Maturity() types.Scalar   { return eq.lastMaturity }
