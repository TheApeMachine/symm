package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
GeometricMean owns exp(mean(log x)).
*/
type GeometricMean struct {
	*core.PrimitiveError

	count float64
	sum   float64
	out   float64
}

func NewGeometricMean() *GeometricMean {
	return &GeometricMean{PrimitiveError: core.NewPrimitiveError()}
}

func (geometricMean *GeometricMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			geometricMean.count++
			geometricMean.sum += math.Log(val)
			geometricMean.out = math.Exp(geometricMean.sum / geometricMean.count)

			if !yield(unsafe.Pointer(&geometricMean.out)) {
				return
			}
		}
	}
}
