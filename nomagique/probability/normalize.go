package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Normalize divides each arrival by the run's total.
*/
type Normalize struct {
	*core.PrimitiveError

	out float64
}

func NewNormalize() *Normalize {
	return &Normalize{PrimitiveError: core.NewPrimitiveError()}
}

func (normalize *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64
		var total float64

		for arriving := range in {
			val := *(*float64)(arriving)
			values = append(values, val)
			total += val
		}

		if total == 0 {
			normalize.Error(core.ErrShape)
			return
		}

		for _, val := range values {
			normalize.out = val / total

			if !yield(unsafe.Pointer(&normalize.out)) {
				return
			}
		}
	}
}
