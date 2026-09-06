package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// Floor owns one scalar transform, including its mathematical domain.
type Floor struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewFloor(left core.Primitive) *Floor { return &Floor{current: store.NewRetained(nil), left: left} }
func (operation *Floor) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Floor(value) }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Floor) Read() any { return core.To[any](operation.current) }
