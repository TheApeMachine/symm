package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
And owns one Boolean operation. Configuration supplies the value a run starts
from. What it hands over is the running conjunction after every arrival.
*/
type And struct {
	*core.PrimitiveError
	acc bool
	out bool
}

func NewAnd(current bool) *And {
	return &And{
		PrimitiveError: core.NewPrimitiveError(),
		acc:            current,
	}
}

func (op *And) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := (*bool)(arriving)
			op.acc = op.acc && *val
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
