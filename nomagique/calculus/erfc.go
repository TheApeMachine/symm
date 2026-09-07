package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Erfc owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Erfc struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewErfc(left core.Primitive) *Erfc { return &Erfc{left: left} }
func (operation *Erfc) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Erfc(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Erfc) Read() any { return core.To[any](operation.current) }
