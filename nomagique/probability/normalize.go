package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Normalize divides an arriving value by a run total.
*/
type Normalize struct {
	*core.PrimitiveError
}

func NewNormalize() core.Primitive {
	return &Normalize{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

		val := values[0]
		total := values[1]

		if total == 0 {
			op.Error(core.ErrDomain)
			return
		}

		for value := range data.NewValue(val / total).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
