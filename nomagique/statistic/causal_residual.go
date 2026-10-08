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
			result.ScoreScale = math.Abs(result.Residual)

			if result.PriorVariance > 0 {
				dispersion := math.Sqrt(result.PriorVariance)

				if dispersion > 2.220446049250313e-16 {
					result.ScoreScale = dispersion
				}
			}

			if result.ScoreScale > 0 {
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
