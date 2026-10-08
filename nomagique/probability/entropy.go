package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Entropy owns -sum(p log p). Zero mass contributes zero;
negative inputs produce a domain error.
*/
type Entropy struct {
	*core.PrimitiveError
	acc float64
}

func NewEntropy() core.Primitive {
	return &Entropy{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			mass := *(*float64)(arriving)

			if mass < 0 {
				op.Error(core.ErrDomain)
				return
			}

			if mass > 0 {
				op.acc -= mass * math.Log(mass)
			}
		}

		for value := range data.NewValue(op.acc).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
