package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Unpack evaluates each arriving replayable group and yields its operands.
// Groups travel as core.Primitive values (as emitted by Batch and Pack).
type Unpack struct{ *core.PrimitiveError }

func NewUnpack() core.Primitive { return &Unpack{PrimitiveError: core.NewPrimitiveError()} }

func (op *Unpack) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			group := *(*core.Primitive)(arriving)

			if group == nil {
				op.Error(core.ErrShape)
				return
			}

			for pointer := range group.Next(nil) {
				if !yield(pointer) {
					return
				}
			}

			if err := op.Error(group.Error()); err != nil {
				return
			}
		}
	}
}
