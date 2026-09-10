package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RMS owns sqrt(sum(x²) / n).
*/
type RMS[U core.Floating] struct {
	core.Base[U, U]
	count  U
	energy U
}

func NewRMS[U core.Floating]() *RMS[U] {
	return &RMS[U]{}
}

func (op *RMS[U]) Write(value U) {
	op.Base.Write(value)
	op.count = 0
	op.energy = 0
}

func (op *RMS[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()
			op.count++
			op.energy += value * value

			if !yield(op.Carrier(U(math.Sqrt(float64(op.energy / op.count))))) {
				return
			}
		}
	}
}
