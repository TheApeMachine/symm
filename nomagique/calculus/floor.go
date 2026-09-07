package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Floor owns one scalar transform, including its mathematical domain.
type Floor struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewFloor(left core.Primitive) *Floor { return &Floor{left: left} }
func (operation *Floor) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Floor(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Floor) Read() any { return core.To[any](operation.current) }
