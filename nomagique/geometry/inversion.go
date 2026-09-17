package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Inversion transforms signed edge sympathy strengths into metric topological distances.
High positive sympathy produces near-zero distance; negative or zero sympathy
produces high-cost ridges.
*/
type Inversion struct {
	*core.PrimitiveError
	distance float64
}

func NewInversion() *Inversion {
	return &Inversion{PrimitiveError: core.NewPrimitiveError()}
}

func (inversion *Inversion) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if inversion.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			edge, isEdge := (*Edge)(arriving), false
			strength := 0.0

			// Detect if arriving is *Edge by reading its Weight
			if arrivingEdge := (*Edge)(arriving); arrivingEdge != nil && arrivingEdge.Weight != nil {
				for ptr := range arrivingEdge.Weight.Next(nil) {
					strength = *(*float64)(ptr)
					isEdge = true
					break
				}
			}

			if !isEdge {
				strength = *(*float64)(arriving)
			}

			dist := core.Unit - strength
			if strength > 0 {
				dist = core.Unit / (core.Unit + strength)
			}

			if isEdge {
				edge.Distance = dist
				if !yield(unsafe.Pointer(edge)) {
					return
				}
				continue
			}

			inversion.distance = dist
			if !yield(unsafe.Pointer(&inversion.distance)) {
				return
			}
		}
	}
}
