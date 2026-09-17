package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Relaxation performs authority-weighted stress descent over edges connecting virtual coordinates.
Positive evidence lowers target distance; negative evidence increases it.
Endpoint authority determines the opposite endpoint's share of displacement.
Hot spots emerge around cells with high maturity/SNR authority.
*/
type Relaxation struct {
	*core.PrimitiveError
	positions map[core.Primitive][2]float64
}

func NewRelaxation() *Relaxation {
	return &Relaxation{
		PrimitiveError: core.NewPrimitiveError(),
		positions:      make(map[core.Primitive][2]float64),
	}
}

func (relaxation *Relaxation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			if relaxation.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			edge := (*Edge)(arriving)
			if edge.Left == nil || edge.Right == nil || edge.Weight == nil {
				if !yield(arriving) {
					return
				}
				continue
			}

			if _, exists := relaxation.positions[edge.Left]; !exists {
				initX, initY := 0.0, 0.0
				if coord, ok := edge.Left.(*Coordinate); ok {
					initX, initY = float64(coord.X), float64(coord.Y)
				}
				relaxation.positions[edge.Left] = [2]float64{initX, initY}
			}

			if _, exists := relaxation.positions[edge.Right]; !exists {
				initX, initY := 0.0, 0.0
				if coord, ok := edge.Right.(*Coordinate); ok {
					initX, initY = float64(coord.X), float64(coord.Y)
				}
				relaxation.positions[edge.Right] = [2]float64{initX, initY}
			}

			leftPos := relaxation.positions[edge.Left]
			rightPos := relaxation.positions[edge.Right]

			strength := 0.0
			for ptr := range edge.Weight.Next(nil) {
				strength = *(*float64)(ptr)
				break
			}

			if edge.Distance == 0 {
				edge.Distance = 1.0 - strength
				if strength > 0 {
					edge.Distance = 1.0 / (1.0 + strength)
				}
			}

			deltaX := rightPos[0] - leftPos[0]
			deltaY := rightPos[1] - leftPos[1]
			currentDist := math.Hypot(deltaX, deltaY)

			if currentDist > 0 && strength != 0 {
				weight := math.Abs(strength)
				force := weight * (currentDist - edge.Distance) / currentDist

				leftPos[0] += force * deltaX * 0.5
				leftPos[1] += force * deltaY * 0.5
				rightPos[0] -= force * deltaX * 0.5
				rightPos[1] -= force * deltaY * 0.5

				relaxation.positions[edge.Left] = leftPos
				relaxation.positions[edge.Right] = rightPos
			}

			if !yield(unsafe.Pointer(edge)) {
				return
			}
		}
	}
}
