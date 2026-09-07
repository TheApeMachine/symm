package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Or owns only its Boolean operation.
type Or struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewOr(seed core.Primitive) *Or { return &Or{seed: seed} }
func (operation *Or) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.seed, in, func(held, value bool) bool { return held || value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Or) Read() any { return core.To[any](operation.current) }
