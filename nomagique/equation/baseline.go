package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
CausalResidual (Equation Tier):
Composes an adaptive Welford state primitive with causal residual centering,
dispersion scaling, and Kish maturity.
Exposes zero-cost property accessors for auxiliary telemetry.
*/
type CausalResidual struct {
	welford       adaptive.WelfordEngine
	hasPrior      bool
	lastPriorMean types.Scalar
	lastResidual  types.Scalar
	lastZScore    types.Scalar
	lastMaturity  types.Scalar
}

func (eq *CausalResidual) Step(x types.Scalar) types.Scalar {
	priorCount := eq.welford.Count()
	priorMean := eq.welford.Mean()
	priorDisp := eq.welford.Dispersion()

	eq.welford.Update(float64(x))

	if priorCount == 0 {
		eq.lastPriorMean = x
		eq.lastResidual = 0.0
		eq.lastZScore = 0.0
		eq.lastMaturity = 0.0
		eq.hasPrior = false

		return 0.0
	}

	eq.hasPrior = true
	eq.lastPriorMean = types.Scalar(priorMean)
	eq.lastResidual = x - eq.lastPriorMean

	disp := priorDisp

	if disp <= 0 {
		disp = math.Abs(float64(eq.lastResidual))
	}

	if disp > 0 {
		eq.lastZScore = eq.lastResidual / types.Scalar(disp)
	} else {
		eq.lastZScore = 0.0
	}

	eq.lastMaturity = types.Scalar(1.0 - 1.0/(priorCount+1.0))

	return eq.lastResidual
}

// Auxiliary readings are zero-cost field accesses:
func (eq *CausalResidual) HasPrior() bool         { return eq.hasPrior }
func (eq *CausalResidual) Mean() types.Scalar     { return types.Scalar(eq.welford.Mean()) }
func (eq *CausalResidual) Baseline() types.Scalar { return eq.lastPriorMean }
func (eq *CausalResidual) Residual() types.Scalar { return eq.lastResidual }
func (eq *CausalResidual) ZScore() types.Scalar   { return eq.lastZScore }
func (eq *CausalResidual) Maturity() types.Scalar { return eq.lastMaturity }
func (eq *CausalResidual) Dispersion() types.Scalar {
	return types.Scalar(eq.welford.Dispersion())
}
func (eq *CausalResidual) Count() float64 { return eq.welford.Count() }

/*
NoiseVariance returns the residual noise power the ZScore normalizes by: the
dispersion before the square root. A consumer derives the scalar SNR from it
without re-deriving the estimator's own noise model.
*/
func (eq *CausalResidual) NoiseVariance() types.Scalar {
	dispersion := eq.Dispersion()

	return dispersion * dispersion
}

// CausalBaseline is an alias for CausalResidual adhering to the monomorphic equation contract.
type CausalBaseline = CausalResidual
