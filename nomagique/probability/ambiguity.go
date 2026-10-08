package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Ambiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
*/
type Ambiguity struct {
	*core.PrimitiveError
	values []float64
	total  float64
}

func NewAmbiguity() core.Primitive {
	return &Ambiguity{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Ambiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			op.values = append(op.values, val)
			op.total += val
		}

		ambiguityVal := 0.0

		if len(op.values) > 1 && op.total > 0 {
			entropy := 0.0

			for _, elem := range op.values {
				probabilityVal := elem / op.total

				if probabilityVal > 0 {
					entropy -= probabilityVal * math.Log(probabilityVal)
				}
			}

			ambiguityVal = entropy / math.Log(float64(len(op.values)))
		}

		for value := range data.NewValue(ambiguityVal).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
