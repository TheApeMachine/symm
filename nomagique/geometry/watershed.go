package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Watershed finds density peaks and assigns basin membership.
Yields basin and peak.
*/
type Watershed struct {
	*core.PrimitiveError
	maxAuthority float64
	peakX        float64
	peakY        float64
	seen         bool
}

func NewWatershed() *Watershed {
	return &Watershed{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Watershed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 3 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 3 {
			op.Error(core.ErrShape)
			return
		}

		coordX := values[0]
		coordY := values[1]
		authority := values[2]

		peak := 0.0

		if !op.seen || authority > op.maxAuthority {
			op.maxAuthority = authority
			op.peakX = coordX
			op.peakY = coordY
			op.seen = true
			peak = 1.0
		}

		quadrant := 0.0

		if coordX >= 0.5 {
			quadrant += 1.0
		}

		if coordY >= 0.5 {
			quadrant += 2.0
		}

		for value := range data.NewValue(quadrant, peak).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
