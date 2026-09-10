package correlation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
FisherView is one correlation mapped through atanh, scored against the
configured causal estimator, then mapped back with tanh. Invalid observations
do not advance the estimator.
*/
type FisherView struct {
	Correlation     float64
	Defined         bool
	Baseline        float64
	Divergence      float64
	PriorCount      float64
	Count           float64
	ZScore          float64
	Variance        float64
	VarianceDefined bool
	HasPrior        bool
}

/*
FisherEstimator transforms admissible scalar correlations before the configured
causal estimator. The estimator supplies recurrence; this composer keeps no
second copy.
*/
type FisherEstimator struct {
	core.Base[float64, FisherView]
	moments  core.Primitive[float64, equation.MomentReading]
	residual *equation.CausalResidual
	atanh    *calculus.Atanh[float64]
	tanh     *calculus.Tanh[float64]
}

func NewFisherEstimator(moments core.Primitive[float64, equation.MomentReading]) *FisherEstimator {
	return &FisherEstimator{
		moments:  moments,
		residual: equation.NewCausalResidual(),
		atanh:    calculus.NewAtanh[float64](),
		tanh:     calculus.NewTanh[float64](),
	}
}

func (op *FisherEstimator) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[FisherView, FisherView]] {
	return func(yield func(core.Primitive[FisherView, FisherView]) bool) {
		for arriving := range in {
			value := arriving.Read()
			view := FisherView{Correlation: value}

			if value > -1 && value < 1 {
				z, err := transport.Evaluate(op.atanh, transport.Values(value))

				if err != nil {
					op.Error(err)
					return
				}

				reading, err := transport.Evaluate(op.moments, transport.Values(z))

				if err != nil {
					op.Error(err)
					return
				}

				scored, err := transport.Evaluate(op.residual, transport.Values(reading))

				if err != nil {
					op.Error(err)
					return
				}

				baseline, err := transport.Evaluate(op.tanh, transport.Values(scored.Baseline))

				if err != nil {
					op.Error(err)
					return
				}

				view.Defined = true
				view.Baseline = baseline
				view.Divergence = scored.Residual
				view.PriorCount = scored.Prior.Count
				view.Count = scored.Count
				view.ZScore = scored.ZScore
				view.Variance = scored.Variance
				view.VarianceDefined = scored.VarianceDefined
				view.HasPrior = scored.HasPrior
			}

			if !yield(op.Carrier(view)) {
				return
			}
		}
	}
}
