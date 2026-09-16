package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Entropy owns -sum(p log p). Zero mass contributes its limiting value zero;
negative inputs retain the logarithm's undefined-domain result.
*/
type Entropy struct {
	*core.PrimitiveError

	acc float64
	out float64
}

func NewEntropy() *Entropy {
	return &Entropy{PrimitiveError: core.NewPrimitiveError()}
}

func (entropy *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			mass := *(*float64)(arriving)
			contribution := 0.0

			if mass != 0 {
				contribution = -mass * math.Log(mass)
			}

			entropy.acc += contribution
			entropy.out = entropy.acc

			if !yield(unsafe.Pointer(&entropy.out)) {
				return
			}
		}
	}
}
