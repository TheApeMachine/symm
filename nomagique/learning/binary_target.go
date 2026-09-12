package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BinaryTarget classifies an increase without inventing a new numeric rule.
*/
type BinaryTarget struct {
	err error
	out float64
}

func NewBinaryTarget() core.Primitive {
	return &BinaryTarget{}
}

func (op *BinaryTarget) Next(
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

			op.out = 0.0

			if sample.Current > sample.Past {
				op.out = 1.0
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *BinaryTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
