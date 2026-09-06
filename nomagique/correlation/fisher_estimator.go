package correlation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
FisherEstimator tracks a correlation's causal history in Fisher coordinates
and reports its baseline back in correlation space.

A correlation cannot be averaged where it lives: its sampling dispersion
collapses toward the ±1 boundaries, so a residual measured in correlation
space is normalized by a scale that is not stationary. The Fisher transform
maps it onto the whole real line where the dispersion is stable; the inverse
maps the resulting baseline back into [-1, 1] so it keeps its documented range.

It is a Chain of three composed stages — Atanh, the causal estimator, and the
readings taken back through Tanh — with no arithmetic of its own.

Degenerate behavior: a saturated |r| >= 1 is outside the transform's domain,
so the estimator is not advanced and the reading holds its prior state.
*/
type FisherEstimator struct {
	transform calculus.Atanh
	invert    calculus.Tanh
	estimator equation.CausalResidual

	defined bool
}

/*
Step transforms the incoming correlation into Fisher space, advances the
causal estimator over it, and returns the residual in Fisher space.
*/
func (fisher *FisherEstimator) Step(correlation types.Number) types.Number {
	fisher.defined = correlation > -1 && correlation < 1

	if !fisher.defined {
		return 0
	}

	return fisher.estimator.Step(fisher.transform.Step(correlation))
}

// Defined reports whether the last correlation was inside the transform's domain.
func (fisher *FisherEstimator) Defined() bool { return fisher.defined }

// HasPrior reports whether the estimator has a causal history to measure against.
func (fisher *FisherEstimator) HasPrior() bool { return fisher.estimator.HasPrior() }

/*
Baseline returns the estimated baseline mapped back into correlation space,
so it keeps the [-1, 1] range a correlation is reported in.
*/
func (fisher *FisherEstimator) Baseline() types.Number {
	return fisher.invert.Step(fisher.estimator.Baseline())
}

// Divergence returns the residual in Fisher space, where it is comparable.
func (fisher *FisherEstimator) Divergence() types.Number {
	return fisher.estimator.Residual()
}

// ZScore returns the standardized departure in Fisher space.
func (fisher *FisherEstimator) ZScore() types.Number { return fisher.estimator.ZScore() }

// Dispersion returns the Fisher-space noise scale.
func (fisher *FisherEstimator) Dispersion() types.Number {
	return fisher.estimator.Dispersion()
}

// NoiseVariance returns the Fisher-space noise power.
func (fisher *FisherEstimator) NoiseVariance() types.Number {
	return fisher.estimator.NoiseVariance()
}

// Count returns how many observations the estimator has retained.
func (fisher *FisherEstimator) Count() float64 { return fisher.estimator.Count() }

// Maturity returns the estimator's Kish maturity.
func (fisher *FisherEstimator) Maturity() types.Number {
	return fisher.estimator.Maturity()
}

var _ types.Node = (*FisherEstimator)(nil)
