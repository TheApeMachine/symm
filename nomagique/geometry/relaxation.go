package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Relaxation performs one weighted stress descent displacement step.
Yields new x, new y, and displacement.
*/
type Relaxation struct {
	*core.PrimitiveError
}

func NewRelaxation() *Relaxation {
	return &Relaxation{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Relaxation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [6]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 6 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 6 {
			op.Error(core.ErrShape)
			return
		}

		currentX := values[0]
		currentY := values[1]
		targetX := values[2]
		targetY := values[3]
		strength := values[4]
		authority := values[5]

		deltaX := targetX - currentX
		deltaY := targetY - currentY
		distance := math.Hypot(deltaX, deltaY)

		displacement := 0.0
		newX := currentX
		newY := currentY

		if distance > 0 && authority > 0 && strength != 0 {
			targetDist := 1.0 - strength

			if strength > 0 {
				targetDist = 1.0 / (1.0 + strength)
			}

			weight := math.Abs(strength)
			force := weight * (distance - targetDist) / distance
			diffX := force * deltaX / authority
			diffY := force * deltaY / authority

			newX += diffX
			newY += diffY
			displacement = diffX*diffX + diffY*diffY
		}

		for value := range data.NewValue(newX, newY, displacement).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
