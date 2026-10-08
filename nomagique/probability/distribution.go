package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Distribution owns confidence, ambiguity, and sharpness over probability readouts.
*/
type Distribution struct {
	*core.PrimitiveError
}

func NewDistribution() core.Primitive {
	return &Distribution{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Distribution) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 2 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 2 {
			op.Error(core.ErrShape)
			return
		}

		confidence := values[0]
		ambiguityVal := values[1]
		sharpness := 1.0 - ambiguityVal

		for value := range data.NewValue(confidence, ambiguityVal, sharpness).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
