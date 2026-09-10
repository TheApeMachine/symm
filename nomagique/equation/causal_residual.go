package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CausalResidualResult measures an observation against the moments that existed
before it. Zero prior dispersion uses |residual| as the score scale, or zero
when the residual is zero.
*/
type CausalResidualResult struct {
	MomentReading
	HasPrior      bool
	Baseline      float64
	PriorVariance float64
	Maturity      float64
	Residual      float64
	ScoreScale    float64
	ZScore        float64
	NoiseVariance float64
}

/*
CausalResidual owns that projection. Recurrence belongs to whoever supplies
the reading.
*/
type CausalResidual struct {
	core.Base[MomentReading, CausalResidualResult]
}

func NewCausalResidual() *CausalResidual {
	return &CausalResidual{}
}

func (op *CausalResidual) Next(
	in iter.Seq[core.Primitive[MomentReading, MomentReading]],
) iter.Seq[core.Primitive[CausalResidualResult, CausalResidualResult]] {
	return func(yield func(core.Primitive[CausalResidualResult, CausalResidualResult]) bool) {
		for arriving := range in {
			reading := arriving.Read()
			result := CausalResidualResult{
				MomentReading: reading,
				HasPrior:      reading.Prior.Count > 0,
				Baseline:      reading.Value,
				Maturity:      1 - 1/(reading.Prior.Count+1),
				NoiseVariance: reading.Variance,
			}

			if result.HasPrior {
				result.Baseline = reading.Prior.Mean
			}

			if reading.Prior.Count > 1 {
				result.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
			}

			result.Residual = reading.Value - result.Baseline
			result.ScoreScale = math.Abs(result.Residual)

			if result.PriorVariance > 0 {
				result.ScoreScale = math.Sqrt(result.PriorVariance)
			}

			if result.ScoreScale > 0 {
				result.ZScore = result.Residual / result.ScoreScale
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
