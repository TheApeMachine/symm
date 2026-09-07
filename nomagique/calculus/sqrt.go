package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Sqrt owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Sqrt struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewSqrt(left core.Primitive) *Sqrt { return &Sqrt{left: left} }
func (operation *Sqrt) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Sqrt(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Sqrt) Read() any { return core.To[any](operation.current) }
