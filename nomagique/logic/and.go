package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// And owns only its Boolean operation.
type And struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewAnd(seed core.Primitive) *And { return &And{current: store.NewRetained(nil), seed: seed} }
func (operation *And) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.seed, in, func(held, value bool) bool { return held && value }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *And) Read() any { return core.To[any](operation.current) }
