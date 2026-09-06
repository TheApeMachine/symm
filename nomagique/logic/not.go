package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Not owns only its Boolean operation.
type Not struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewNot(seed core.Primitive) *Not { return &Not{current: store.NewRetained(nil), seed: seed} }
func (operation *Not) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.seed, in, func(held, value bool) bool { return !value }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Not) Read() any { return core.To[any](operation.current) }
