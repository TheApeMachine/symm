package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DirectionalTarget composes a finite nonnegative deadband and the sign of a delta.
Deadband is configuration of this target.
*/
type DirectionalTarget struct {
	err      error
	Deadband float64
	out      float64
}

func NewDirectionalTarget(deadband float64) core.Primitive {
	return &DirectionalTarget{Deadband: deadband}
}

func (op *DirectionalTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) || math.IsNaN(op.Deadband) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) || math.IsInf(op.Deadband, 0) ||
				op.Deadband < 0 {
				op.Error(core.ErrDomain)
				return
			}

			delta := sample.Current - sample.Past
			magnitude := math.Abs(delta)
			op.out = 0.0

			if magnitude > op.Deadband {
				op.out = math.Copysign(1, delta)
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *DirectionalTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
