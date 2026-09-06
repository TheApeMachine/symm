package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// Atanh owns only its numeric operation. The configured left source remains
// connected; neither a yielded result nor an exhausted run can replace it.
type Atanh struct {
	core.PrimitiveError
	left    core.Primitive
	current core.Primitive
}

func NewAtanh(left core.Primitive) *Atanh { return &Atanh{current: store.NewRetained(nil), left: left} }
func (operation *Atanh) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value float64) float64 { return math.Atanh(value) }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Atanh) Read() any { return core.To[any](operation.current) }
