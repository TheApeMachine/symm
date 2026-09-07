package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Not owns only its Boolean operation.
type Not struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewNot(seed core.Primitive) *Not { return &Not{seed: seed} }
func (operation *Not) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.seed, in, func(held, value bool) bool { return !value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Not) Read() any { return core.To[any](operation.current) }
