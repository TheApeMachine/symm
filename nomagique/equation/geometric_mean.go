package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
GeometricMean owns exp(mean(log x)). Zero and invalid-domain behavior come from
those operations.
*/
type GeometricMean[U core.Floating] struct {
	core.Base[U, U]
	count U
	sum   U
}

func NewGeometricMean[U core.Floating]() *GeometricMean[U] {
	return &GeometricMean[U]{}
}

func (op *GeometricMean[U]) Write(value U) {
	op.Base.Write(value)
	op.count = 0
	op.sum = 0
}

func (op *GeometricMean[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			op.count++
			op.sum += U(math.Log(float64(arriving.Read())))

			if !yield(op.Carrier(U(math.Exp(float64(op.sum / op.count))))) {
				return
			}
		}
	}
}
