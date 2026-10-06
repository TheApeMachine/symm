package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fisher owns the Fisher-z normal approximation. Each arrival is
[3]float64{correlation, support, searchCount}; it yields
[6]float64{defined, pValue, z, standardError, searchAdjustedPValue, hasSearch}.
Undefined inputs yield defined=0 and zeroed tails.
*/
type Fisher struct {
	*core.PrimitiveError
	out [6]float64
}

func NewFisher() core.Primitive {
	return &Fisher{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Fisher) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := *(*[3]float64)(arriving)
			correlation, support, searchCount := sample[0], sample[1], sample[2]
			op.out = [6]float64{}

			if searchCount >= 1 {
				op.out[5] = 1
			}

			if support > 3 && math.Abs(correlation) <= 1 {
				degrees := math.Sqrt(support - 3)
				z := math.Atanh(correlation) * degrees
				p := math.Erfc(math.Abs(z) / math.Sqrt2)

				op.out[0] = 1
				op.out[1] = p
				op.out[2] = z
				op.out[3] = 1.0 / degrees

				if op.out[5] == 1 {
					adj := p * searchCount

					if adj > 1.0 {
						adj = 1.0
					}

					op.out[4] = adj
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
