package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Or owns one Boolean operation. Configuration supplies the value a run starts
from. What it hands over is the running disjunction after every arrival.
*/
type Or struct {
	*core.PrimitiveError
	acc bool
	out bool
}

func NewOr(current bool) *Or {
	return &Or{
		PrimitiveError: core.NewPrimitiveError(),
		acc:            current,
	}
}

func (op *Or) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := (*bool)(arriving)
			op.acc = op.acc || *val
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
