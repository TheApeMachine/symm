package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// IsNaN is the numerical undefinedness predicate, not a fallback policy.
type IsNaN struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewIsNaN() *IsNaN {
	return &IsNaN{current: store.NewRetained(nil), seed: transport.NewIO(core.From(false))}
}
func (predicate *IsNaN) Next(in core.Primitive) core.Primitive {
	result := core.Yield(predicate.seed, in, func(_ bool, v float64) bool { return math.IsNaN(v) }, predicate)
	transport.NewDiscard().Next(transport.NewApply(predicate.current, transport.NewIO(result)))
	return result
}
func (predicate *IsNaN) Read() any { return core.To[any](predicate.current) }
