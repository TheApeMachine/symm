package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Maximum owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Maximum struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewMaximum(left core.Primitive) *Maximum {
	return &Maximum{left: left}
}
func (operation *Maximum) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Max(held, value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Maximum) Read() any { return core.To[any](operation.current) }
