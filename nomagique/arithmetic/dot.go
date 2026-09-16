package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Dot owns the inner product of two equal-length vectors.
*/
type Dot struct {
	*core.PrimitiveError

	out float64
}

func NewDot() *Dot {
	return &Dot{PrimitiveError: core.NewPrimitiveError()}
}

func (dot *Dot) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if len(pair.Left) != len(pair.Right) {
				dot.Error(core.ErrShape)
				continue
			}

			dot.out = 0.0
			for index, value := range pair.Left {
				dot.out += value * pair.Right[index]
			}

			if !yield(unsafe.Pointer(&dot.out)) {
				return
			}
		}
	}
}
