package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DeltaTarget returns the observed current-minus-past difference.
*/
type DeltaTarget struct {
	err error
	out float64
}

func NewDeltaTarget() core.Primitive {
	return &DeltaTarget{}
}

func (op *DeltaTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) {
				op.Error(core.ErrDomain)
				return
			}

			op.out = sample.Current - sample.Past

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *DeltaTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
