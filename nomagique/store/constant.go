package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Constant replaces each delivered value with a configured Primitive. Mapping
// this replacement to one, then Add, is counting; no Count kernel is needed.
type Constant struct {
	core.PrimitiveError
	value   core.Primitive
	seed    core.Primitive
	current core.Primitive
}

func NewConstant(value core.Primitive) *Constant {
	return &Constant{current: NewRetained(nil), value: value, seed: transport.NewIO(value)}
}
func (constant *Constant) Next(in core.Primitive) core.Primitive {
	result := core.Yield(constant.seed, in, func(held core.Primitive, _ core.Primitive) core.Primitive { return held }, constant)
	transport.NewDiscard().Next(transport.NewApply(constant.current, transport.NewIO(result)))
	return result
}
func (constant *Constant) Read() any { return core.To[any](constant.current) }
