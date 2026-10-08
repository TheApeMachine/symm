package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Overlap owns the normalized inner product and distance between coordinate pairs.
Yields overlap (dot product), affinity, and distance.
*/
type Overlap struct {
	*core.PrimitiveError
}

func NewOverlap() *Overlap {
	return &Overlap{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Overlap) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [4]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 4 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 4 {
			op.Error(core.ErrShape)
			return
		}

		firstX := values[0]
		firstY := values[1]
		secondX := values[2]
		secondY := values[3]

		dot := firstX*secondX + firstY*secondY
		firstNorm := math.Hypot(firstX, firstY)
		secondNorm := math.Hypot(secondX, secondY)
		affinity := 0.0

		if firstNorm > 0 && secondNorm > 0 {
			affinity = dot / (firstNorm * secondNorm)
		}

		distance := math.Hypot(secondX-firstX, secondY-firstY)

		for value := range data.NewValue(dot, affinity, distance).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
