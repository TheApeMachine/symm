package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sum owns the running arithmetic sum.
*/
type Sum struct {
	*core.PrimitiveError
	total float64
	out   float64
}

func NewSum() *Sum {
	return &Sum{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			op.total += val
			op.out = op.total

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
