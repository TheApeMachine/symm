package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Sign discards magnitude. NaN remains undefined; infinities have defined signs.
type Sign struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewSign(left core.Primitive) *Sign { return &Sign{left: left} }
func (operation *Sign) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		operation.left,
		in,
		func(_, value float64) float64 {
			if math.IsNaN(value) {
				return value
			}
			if value == 0 {
				return value
			}
			return math.Copysign(1, value)
		},
		operation,
	)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Sign) Read() any { return core.To[any](operation.current) }
