package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IdentityTarget selects the finite current value.
*/
type IdentityTarget struct {
	err error
	out float64
}

func NewIdentityTarget() core.Primitive {
	return &IdentityTarget{}
}

func (op *IdentityTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsInf(sample.Current, 0) {
				op.Error(core.ErrDomain)
				return
			}

			op.out = sample.Current

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *IdentityTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
