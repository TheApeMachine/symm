package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// Sign discards magnitude. NaN remains undefined; infinities have defined signs.
type Sign struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewSign(left core.Primitive) *Sign { return &Sign{current: store.NewRetained(nil), left: left} }
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
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Sign) Read() any { return core.To[any](operation.current) }
