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
	return &Sum{PrimitiveError: core.NewPrimitiveError()}
}

func (sum *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			sum.total += val
			sum.out = sum.total

			if !yield(unsafe.Pointer(&sum.out)) {
				return
			}
		}
	}
}
