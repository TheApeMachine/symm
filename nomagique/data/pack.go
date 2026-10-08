package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Pack freezes a run into a replayable Value, copying arrivals while valid.
// The optional arity describes a complete group: fewer arrivals yield no group,
// excess arrivals are a shape error. Without an arity any nonempty run is packed.
// This lets a composition wait for both independently available distributions
// without converting an absent side into a zero-valued distribution.
type Pack[T any] struct {
	*core.PrimitiveError
	arity int
}

func NewPack[T any](arity ...int) core.Primitive {
	op := &Pack[T]{PrimitiveError: core.NewPrimitiveError()}
	if len(arity) > 1 {
		op.Error(core.ErrShape)
		return op
	}
	if len(arity) == 1 {
		op.arity = arity[0]
		if op.arity <= 0 {
			op.Error(core.ErrShape)
		}
	}
	return op
}

func (op *Pack[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}
		var values []T
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			values = append(values, *(*T)(arriving))
		}
		if op.arity > 0 && len(values) > op.arity {
			op.Error(core.ErrShape)
			return
		}
		if len(values) == 0 || len(values) < op.arity {
			return
		}
		var group core.Primitive = NewValue(values...)
		yield(unsafe.Pointer(&group))
	}
}
