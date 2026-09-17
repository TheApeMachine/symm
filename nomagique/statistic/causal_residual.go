package statistic

import (
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
	*core.PrimitiveError

	out CausalResidualResult
}

func NewCausalResidual() *CausalResidual {
	return &CausalResidual{PrimitiveError: core.NewPrimitiveError()}
}

func (causalResidual *CausalResidual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

				if dispersion > core.Epsilon {
					result.ScoreScale = dispersion
				}
			}

			if result.ScoreScale > 0 {
				result.ZScore = result.Residual / result.ScoreScale
			}

			causalResidual.out = result

			if !yield(unsafe.Pointer(&causalResidual.out)) {
				return
			}
		}
	}
}

/*
ResidualBaseline yields the causal baseline of a residual reading.
*/
type ResidualBaseline struct {
	*core.PrimitiveError

	out float64
}

func NewResidualBaseline() *ResidualBaseline {
	return &ResidualBaseline{PrimitiveError: core.NewPrimitiveError()}
}

func (residualBaseline *ResidualBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			result := (*CausalResidualResult)(arriving)
			residualBaseline.out = result.Baseline

			if !yield(unsafe.Pointer(&residualBaseline.out)) {
				return
			}
		}
	}
}

/*
ResidualDivergence yields the causal residual of a residual reading.
*/
type ResidualDivergence struct {
	*core.PrimitiveError

	out float64
}

func NewResidualDivergence() *ResidualDivergence {
	return &ResidualDivergence{PrimitiveError: core.NewPrimitiveError()}
}

func (residualDivergence *ResidualDivergence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			result := (*CausalResidualResult)(arriving)
			residualDivergence.out = result.Residual

			if !yield(unsafe.Pointer(&residualDivergence.out)) {
				return
			}
		}
	}
}

/*
ResidualZScore yields the standardized causal residual.
*/
type ResidualZScore struct {
	*core.PrimitiveError

	out float64
}

func NewResidualZScore() *ResidualZScore {
	return &ResidualZScore{PrimitiveError: core.NewPrimitiveError()}
}

func (residualZScore *ResidualZScore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			result := (*CausalResidualResult)(arriving)
			residualZScore.out = result.ZScore

			if !yield(unsafe.Pointer(&residualZScore.out)) {
				return
			}
		}
	}
}
