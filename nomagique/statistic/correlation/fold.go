package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fold folds admitted peer correlations into one cohort summary.
*/
type Fold struct {
	*core.PrimitiveError

	cohort core.Primitive
}

func NewFold() *Fold {
	return &Fold{PrimitiveError: core.NewPrimitiveError(), cohort: NewCohort()}
}

func (fold *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return fold.cohort.Next(in)
}
