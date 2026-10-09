package statistic

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CausalResidualResult measures an observation against the moments that existed
before it.
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
CausalResidual owns that projection.
*/
type CausalResidual struct {
	err error
	out CausalResidualResult
}

func NewCausalResidual() core.Primitive {
	return &CausalResidual{}
}

func (op *CausalResidual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			reading := *(*MomentReading)(arriving)
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

			// The z-score needs a positive prior dispersion; a lone residual is
			// not its own scale. ScoreScale stays 0 while it is undefined.
			if result.PriorVariance > 0 {
				result.ScoreScale = math.Sqrt(result.PriorVariance)
				result.ZScore = result.Residual / result.ScoreScale
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *CausalResidual) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
