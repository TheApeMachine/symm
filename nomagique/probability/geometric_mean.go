package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
GeometricMean owns exp(mean(log x)).
*/
type GeometricMean struct {
	*core.PrimitiveError
	count float64
	sum   float64
}

func NewGeometricMean() core.Primitive {
	return &GeometricMean{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *GeometricMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			if val <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.count++
			op.sum += math.Log(val)
		}

		if op.count == 0 {
			return
		}

		for value := range data.NewValue(math.Exp(op.sum / op.count)).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
