package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ambiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
*/
type Ambiguity struct {
	*core.PrimitiveError

	out float64
}

func NewAmbiguity() *Ambiguity {
	return &Ambiguity{PrimitiveError: core.NewPrimitiveError()}
}

func (ambiguity *Ambiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64
		var total float64

		for arriving := range in {
			val := *(*float64)(arriving)
			values = append(values, val)
			total += val
		}

		if len(values) <= 1 {
			ambiguity.out = 0
			yield(unsafe.Pointer(&ambiguity.out))
			return
		}

		if total == 0 {
			ambiguity.out = 0
			yield(unsafe.Pointer(&ambiguity.out))
			return
		}

		entropy := 0.0

		for _, val := range values {
			p := val / total

			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		ambiguity.out = entropy / math.Log(float64(len(values)))
		yield(unsafe.Pointer(&ambiguity.out))
	}
}
