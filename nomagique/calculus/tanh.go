package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Tanh owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Tanh struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewTanh(left core.Primitive) *Tanh { return &Tanh{left: left} }
func (operation *Tanh) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Tanh(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Tanh) Read() any { return core.To[any](operation.current) }
