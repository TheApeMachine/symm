package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Peak owns one delivery's maximum absolute ordinate and its original point.
Each point arrival is [2]float64{x, y}; after the run it yields
[3]float64{index, x, y}. An empty run yields nothing.
*/
type Peak struct {
	*core.PrimitiveError
	out [3]float64
}

func NewPeak() core.Primitive {
	return &Peak{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Peak) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		seen := false
		index := 0.0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			point := *(*[2]float64)(arriving)
			magnitude := math.Abs(point[1])

			if !seen || magnitude > math.Abs(op.out[2]) {
				op.out = [3]float64{index, point[0], point[1]}
				seen = true
			}

			index++
		}

		if !seen {
			return
		}

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}
