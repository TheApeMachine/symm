package arithmetic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Finite reports whether every scalar in the vector is finite.
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
			values := *(*[]float64)(arriving)
			valid := true

			for _, value := range values {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					valid = false
					break
				}
			}

			finite.out = valid
			if !yield(unsafe.Pointer(&finite.out)) {
				return
			}
		}
	}
}
