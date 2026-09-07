package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Reciprocal owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Reciprocal struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewReciprocal(left core.Primitive) *Reciprocal {
	return &Reciprocal{left: left}
}
func (operation *Reciprocal) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return 1 / value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Reciprocal) Read() any { return core.To[any](operation.current) }
