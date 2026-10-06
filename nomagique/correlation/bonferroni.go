package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Bonferroni owns min(p * candidates, 1), the union bound. Each arrival is
[2]float64{p, candidates}; it yields *float64.
*/
type Bonferroni struct {
	*core.PrimitiveError
	out float64
}

func NewBonferroni() core.Primitive {
	return &Bonferroni{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Bonferroni) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := *(*[2]float64)(arriving)
			val := input[0] * input[1]

			if val > 1.0 {
				val = 1.0
			}

			op.out = val

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
