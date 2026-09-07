package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Exp owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Exp struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewExp(left core.Primitive) *Exp { return &Exp{left: left} }
func (operation *Exp) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Exp(value) }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Exp) Read() any { return core.To[any](operation.current) }
