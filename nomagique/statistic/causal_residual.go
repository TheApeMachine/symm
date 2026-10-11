package statistic

import (
	"errors"
	"iter"
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

			result.Residual = reading.Value - result.Baseline

			// The z-score needs a verified prior dispersion (at least MinimumPrior
			// observations, positive dispersion, and a scale not negligible next to
			// baseline or value); ScoreScale stays 0 while it is undefined.
			if scale, ok := core.PriorScale(reading.Prior.Count, reading.Prior.M2, reading.Value, result.Baseline); ok {
				result.ScoreScale = scale
				result.PriorVariance = scale * scale
				result.ZScore = result.Residual / scale
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
