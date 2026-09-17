package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
History feeds the cohort's signed correlation to the Fisher-space causal
estimator, giving the measurement its baseline, divergence, and z-score.
*/
type History struct {
	*core.PrimitiveError

	estimator core.Primitive
}

func NewHistory() *History {
	return &History{PrimitiveError: core.NewPrimitiveError(), estimator: NewFisherEstimator()}
}

func (history *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return history.estimator.Next(in)
}
