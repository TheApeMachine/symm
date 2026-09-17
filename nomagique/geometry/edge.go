package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Edge is a Primitive connecting two nodes across a weighted channel.
It composes transport.IO and a weight Primitive to route or measure relationships.
*/
type Edge struct {
	*core.PrimitiveError
	Left      core.Primitive
	Right     core.Primitive
	Weight    core.Primitive
	Authority [2]float64
	Distance  float64
	Basin     [2]int
	Border    bool
	pipe      core.Primitive
}

func NewEdge(
	left core.Primitive,
	right core.Primitive,
	weight core.Primitive,
) *Edge {
	edge := &Edge{
		PrimitiveError: core.NewPrimitiveError(),
		Left:           left,
		Right:          right,
		Weight:         weight,
	}

	if left != nil && right != nil && weight != nil {
		edge.pipe = transport.NewIO[any](left, transport.NewIO[any](weight, right))
	}

	return edge
}

func (edge *Edge) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			if edge.Weight != nil {
				for out := range edge.Weight.Next(nil) {
					if !yield(out) {
						return
					}
				}
			}

			return
		}

		if edge.pipe != nil {
			for out := range edge.pipe.Next(in) {
				if !yield(out) {
					return
				}
			}
		}
	}
}
