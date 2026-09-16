package logic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Finite owns the finiteness predicate. What it hands over is whether each
arrival is a finite number.
*/
type Finite struct {
	*core.PrimitiveError

	out bool
}

func NewFinite() *Finite {
	return &Finite{PrimitiveError: core.NewPrimitiveError()}
}

func (finite *Finite) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			number := *in
			finite.out = !math.IsNaN(number) && !math.IsInf(number, 0)

			if !yield(unsafe.Pointer(&finite.out)) {
				return
			}
		}
	}
}
