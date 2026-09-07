package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// And owns only its Boolean operation.
type And struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewAnd(seed core.Primitive) *And { return &And{seed: seed} }
func (operation *And) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.seed, in, func(held, value bool) bool { return held && value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *And) Read() any { return core.To[any](operation.current) }
