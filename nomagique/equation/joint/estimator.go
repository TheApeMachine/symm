package joint

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/vector"
)

/*
Input is a log-space observation vector. Each coordinate has its own estimator.
*/
type Input struct {
	Values []float64
}

/*
Result is per-channel log-moment views and the diagonal standardized-energy mean.
*/
type Result struct {
	Channels   []equation.CausalResidualResult
	Energies   []float64
	SNR        float64
	SNRDefined bool
}

/*
Estimator applies one configured moment estimator per coordinate.
*/
type Estimator struct {
	core.Base[Input, Result]
	apply *vector.Apply[float64, equation.MomentReading]
	view  *equation.LogMomentView
}

func NewEstimator(estimators ...core.Primitive[float64, equation.MomentReading]) *Estimator {
	return &Estimator{
		apply: vector.NewApply(estimators...),
		view:  equation.NewLogMomentView(),
	}
}

func (op *Estimator) Next(
	in iter.Seq[core.Primitive[Input, Input]],
) iter.Seq[core.Primitive[Result, Result]] {
	return func(yield func(core.Primitive[Result, Result]) bool) {
		for arriving := range in {
			values := arriving.Read().Values
			readings := make([]equation.MomentReading, 0, len(values))

			for reading := range op.apply.Next(func(yield func(core.Primitive[float64, float64]) bool) {
				carrier := &core.Carrier[float64]{}

				for _, value := range values {
					if !yield(carrier.Carrier(value)) {
						return
					}
				}
			}) {
				readings = append(readings, reading.Read())
			}

			if err := op.apply.Error(); err != nil {
				op.Error(err)
				return
			}

			var channels []equation.CausalResidualResult
			var energies []float64

			for _, reading := range readings {
				view := equation.CausalResidualResult{}

				for out := range op.view.Next(func(yield func(core.Primitive[equation.MomentReading, equation.MomentReading]) bool) {
					carrier := &core.Carrier[equation.MomentReading]{}
					yield(carrier.Carrier(reading))
				}) {
					view = out.Read()
				}

				channels = append(channels, view)

				if view.ScoreScale > 0 {
					energies = append(energies, view.ZScore*view.ZScore)
				}
			}

			result := Result{Channels: channels, Energies: energies}

			if len(energies) > 0 {
				total := 0.0

				for _, energy := range energies {
					total += energy
				}

				result.SNR = total / float64(len(energies))
				result.SNRDefined = true
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
