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
type causalResidualState struct {
	welford       adaptive.WelfordEngine
	window        adaptive.Window
	hasPrior      bool
	lastPriorMean types.Scalar
	lastResidual  types.Scalar
	lastZScore    types.Scalar
	lastMaturity  types.Scalar
}

type CausalResidual struct {
	types.Guard
	Key func() string

	welford       adaptive.WelfordEngine
	window        adaptive.Window
	hasPrior      bool
	lastPriorMean types.Scalar
	lastResidual  types.Scalar
	lastZScore    types.Scalar
	lastMaturity  types.Scalar

	states map[string]*causalResidualState
	active *causalResidualState
}

func (eq *CausalResidual) key() string {
	if eq.Key != nil {
		return eq.Key()
	}

	return ""
}

func (eq *CausalResidual) resolveState() *causalResidualState {
	activeKey := eq.key()

	if activeKey != "" {
		if eq.states == nil {
			eq.states = make(map[string]*causalResidualState)
		}

		st, found := eq.states[activeKey]

		if !found {
			st = &causalResidualState{}
			eq.states[activeKey] = st
		}

		return st
	}

	return nil
}

func (eq *CausalResidual) Step(x types.Scalar) types.Scalar {
	state := eq.resolveState()
	eq.active = state

	if state != nil {
		if !eq.Fresh() {
			return state.lastResidual
		}

		priorCount := state.welford.Count()
		priorMean := state.welford.Mean()
		priorDisp := state.welford.Dispersion()

		state.welford.Update(float64(x))

		if priorCount == 0 {
			state.lastPriorMean = x
			state.lastResidual = 0.0
			state.lastZScore = 0.0
			state.lastMaturity = 0.0
			state.hasPrior = false
			state.window.Step(float64(x))

			return 0.0
		}

		state.hasPrior = true
		state.lastPriorMean = types.Scalar(priorMean)
		state.lastResidual = x - state.lastPriorMean

		disp := priorDisp

		if disp <= 0 {
			disp = math.Abs(float64(state.lastResidual))
		}

		if disp > 0 {
			state.lastZScore = state.lastResidual / types.Scalar(disp)
		} else {
			state.lastZScore = 0.0
		}

		state.lastMaturity = types.Scalar(1.0 - 1.0/(priorCount+1.0))

		state.window.Step(float64(x))
		state.welford.Shed(state.window.ShedRatio())

		return state.lastResidual
	}

	if !eq.Fresh() {
		return eq.lastResidual
	}

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
		eq.window.Step(float64(x))

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

	eq.window.Step(float64(x))
	eq.welford.Shed(eq.window.ShedRatio())

	return eq.lastResidual
}

// Auxiliary readings are zero-cost field accesses:
func (eq *CausalResidual) HasPrior() bool {
	if eq.active != nil {
		return eq.active.hasPrior
	}

	return eq.hasPrior
}

func (eq *CausalResidual) Mean() types.Scalar {
	if eq.active != nil {
		return types.Scalar(eq.active.welford.Mean())
	}

	return types.Scalar(eq.welford.Mean())
}

func (eq *CausalResidual) Baseline() types.Scalar {
	if eq.active != nil {
		return eq.active.lastPriorMean
	}

	return eq.lastPriorMean
}

func (eq *CausalResidual) Residual() types.Scalar {
	if eq.active != nil {
		return eq.active.lastResidual
	}

	return eq.lastResidual
}

func (eq *CausalResidual) ZScore() types.Scalar {
	if eq.active != nil {
		return eq.active.lastZScore
	}

	return eq.lastZScore
}

func (eq *CausalResidual) Maturity() types.Scalar {
	if eq.active != nil {
		return eq.active.lastMaturity
	}

	return eq.lastMaturity
}

func (eq *CausalResidual) Dispersion() types.Scalar {
	if eq.active != nil {
		return types.Scalar(eq.active.welford.Dispersion())
	}

	return types.Scalar(eq.welford.Dispersion())
}

func (eq *CausalResidual) Count() float64 {
	if eq.active != nil {
		return eq.active.welford.Count()
	}

	return eq.welford.Count()
}

/*
Support, Divergence and NoiseVariance state the confidence this estimate
carries, so a terminal Projection can declare the facts a signal-to-noise
ratio is derived from without deriving it a second time.
*/
func (eq *CausalResidual) Support() float64         { return eq.Count() }
func (eq *CausalResidual) Divergence() types.Scalar { return eq.Residual() }

/*
Readings publishes the standardization this equation performed: the baseline
it measured against, the distance from it, and that distance in units of the
estimate's own dispersion. All three are undefined until a prior exists.
*/
func (eq *CausalResidual) Readings() []types.Reading {
	hasPrior := eq.HasPrior()

	return []types.Reading{
		{
			Label:     "baseline",
			Unit:      "dimensionless",
			Timescale: "instantaneous",
			Value:     eq.Baseline(),
			Defined:   hasPrior,
		},
		{
			Label:     "mean",
			Unit:      "dimensionless",
			Timescale: "instantaneous",
			Value:     eq.Mean(),
			Defined:   hasPrior,
		},
		{
			Label:     "divergence",
			Unit:      "dimensionless",
			Timescale: "instantaneous",
			Value:     eq.Residual(),
			Defined:   hasPrior,
		},
		{
			Label:     "zscore",
			Unit:      "dimensionless",
			Timescale: "instantaneous",
			Value:     eq.ZScore(),
			Defined:   hasPrior,
		},
	}

}

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
