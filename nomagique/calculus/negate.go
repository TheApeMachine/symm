package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Negate owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Negate struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewNegate(left core.Primitive) *Negate {
	return &Negate{left: left}
}
func (operation *Negate) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return -value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Negate) Read() any { return core.To[any](operation.current) }
