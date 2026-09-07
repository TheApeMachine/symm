package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Absolute owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Absolute struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewAbsolute(left core.Primitive) *Absolute {
	return &Absolute{left: left}
}
func (operation *Absolute) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Abs(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Absolute) Read() any { return core.To[any](operation.current) }
