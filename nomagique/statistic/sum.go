package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Sum owns the running arithmetic sum. Arriving values are added to the running
total in stream order and the updated sum is yielded through the output run.
*/
type Sum struct {
	*core.PrimitiveError
}

/*
NewSum creates the running arithmetic sum primitive.
*/
func NewSum() core.Primitive {
	return &Sum{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var out float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			out += *(*float64)(arriving)
		}

		for value := range data.NewValue(out).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
