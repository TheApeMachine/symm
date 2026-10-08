package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Normalize scales coordinate pairs to unit energy.
Yields normalized x, normalized y, and norm.
*/
type Normalize struct {
	*core.PrimitiveError
}

func NewNormalize() *Normalize {
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

		coordinateX := values[0]
		coordinateY := values[1]

		norm := math.Hypot(coordinateX, coordinateY)
		normalizedX := coordinateX
		normalizedY := coordinateY

		if norm > 0 {
			normalizedX = coordinateX / norm
			normalizedY = coordinateY / norm
		}

		for value := range data.NewValue(normalizedX, normalizedY, norm).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
